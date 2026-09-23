package gui

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// SPEC §8.3's DEVICE LEG (F-449 stage 4): receive and change 0..2 derived by
// THIS firmware, for a wallet THIS firmware's composer built, equal the
// addresses LIANA ITSELF recorded for the same wallet -- Liana v15.0's own
// `LianaDescriptor` receive/change output, vendored from the Rust primary as
// md/testdata/liana_cases.json (stage 3, pinned by md/liana_test.go). The
// md leg of the same three-way equality is Rust's (stage 1b); the device-to-md
// leg is TestEveryKeyedVectorReachesAnAddress over the two keyed kind-1 vectors.
//
// THROUGH THE PRODUCTION PATH, end to end: md.ComposeWithUnspendable (the call
// composerArtifactsFor makes), Bind, Chunks, then complexAddressSource -- the
// gated entry every screen uses. No step is a test double.

type lianaDeviceCase struct {
	Name         string   `json:"name"`
	Accepted     bool     `json:"accepted"`
	LeafTLVHex   []string `json:"leaf_tlv_hex"`
	Descriptor   string   `json:"descriptor_with_checksum"`
	LianaReceive []string `json:"liana_receive"`
	LianaChange  []string `json:"liana_change"`
}

// lianaCaseLists is each ACCEPTED case as the composer path list that lowers to
// its tree. Every accepted case is composable -- a flat or right-spined tree of
// multi_a / pk leaves -- which is itself a fact this test pins: a sixth accepted
// case with no entry here fails rather than being skipped.
var lianaCaseLists = map[string]md.PathList{
	"preset-kofn-recovery-tr": {Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3}}, lianaLocked(1, 1, 26280)}},
	"preset-tiered-recovery-tr": {Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2}}, lianaLocked(1, 2, 26280)}},
	"same-seed-two-paths-tr": {Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3}}, lianaLocked(1, 1, 26280)}},
	"X19-tr-kofn-nums-older5": {Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 3}}, lianaLocked(1, 1, 5)}},
	"nested-2of2-two-recoveries-tr": {Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 2, N: 2}}, lianaLocked(1, 1, 26280), lianaLocked(1, 1, 52560)}},
}

func lianaLocked(k, n uint8, blocks uint32) md.SpendPath {
	return md.SpendPath{Keys: &md.KeySet{K: k, N: n}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: blocks}}
}

var lianaOriginRE = regexp.MustCompile(`\[([0-9a-f]{8})((?:/[0-9]+'?)+)\]`)

// lianaCaseChunks builds the case's KEYED md1 through the composer, at `kind`.
func lianaCaseChunks(t *testing.T, c lianaDeviceCase, kind md.UnspendableKind) []string {
	t.Helper()
	list, ok := lianaCaseLists[c.Name]
	if !ok {
		t.Fatalf("accepted case %s has no composer path list -- add one, never skip it", c.Name)
	}
	origins := lianaOriginRE.FindAllStringSubmatch(c.Descriptor, -1)
	if len(origins) != len(c.LeafTLVHex) {
		t.Fatalf("%s: %d origins for %d leaf keys", c.Name, len(origins), len(c.LeafTLVHex))
	}
	declared := make([]*md.SlotOrigin, len(origins))
	pubs := map[uint8][65]byte{}
	fps := map[uint8][4]byte{}
	for i, o := range origins {
		var fp [4]byte
		if _, err := hex.Decode(fp[:], []byte(o[1])); err != nil {
			t.Fatal(err)
		}
		var comps []md.PathComponent
		for _, p := range strings.Split(strings.TrimPrefix(o[2], "/"), "/") {
			hard := strings.HasSuffix(p, "'")
			v, err := strconv.ParseUint(strings.TrimSuffix(p, "'"), 10, 32)
			if err != nil {
				t.Fatal(err)
			}
			comps = append(comps, md.PathComponent{Hardened: hard, Value: uint32(v)})
		}
		declared[i] = &md.SlotOrigin{Origin: comps, Fingerprint: fp, FpPresent: true}
		raw, err := hex.DecodeString(c.LeafTLVHex[i])
		if err != nil || len(raw) != 65 {
			t.Fatalf("%s: leaf %d TLV: %v (len %d)", c.Name, i, err, len(raw))
		}
		var b [65]byte
		copy(b[:], raw)
		pubs[uint8(i)] = b
		fps[uint8(i)] = fp
	}
	composed, err := md.ComposeWithUnspendable(list, declared, kind)
	if err != nil {
		t.Fatalf("%s: compose: %v", c.Name, err)
	}
	if err := composed.Bind(pubs, fps); err != nil {
		t.Fatalf("%s: Bind: %v", c.Name, err)
	}
	chunks, err := composed.Chunks()
	if err != nil {
		t.Fatalf("%s: Chunks: %v", c.Name, err)
	}
	return chunks
}

// TestDeviceDerivesLianasOwnAddressesForKind1 is SPEC §8.3's device leg.
//
// Mutations, each measured: taprootInternalKey's Liana arm returning
// address.NUMSInternalKey() (§7a row 1) fails every case at receive 0;
// lianaInternalKey deriving under the wallet's use-site instead of <0;1>
// is inert here (every wallet is <0;1>, which is §6 row 2's point); the
// Range End set to 2 fails at change 0.
func TestDeviceDerivesLianasOwnAddressesForKind1(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "liana_cases.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: %v", err)
	}
	var cases []lianaDeviceCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, c := range cases {
		if !c.Accepted {
			continue
		}
		accepted++
		t.Run(c.Name, func(t *testing.T) {
			if len(c.LianaReceive) != 3 || len(c.LianaChange) != 3 {
				t.Fatalf("the case records %d receive / %d change addresses, want 3 and 3",
					len(c.LianaReceive), len(c.LianaChange))
			}
			chunks := lianaCaseChunks(t, c, md.UnspendableLiana)
			_, keys, err := md.ExpandWalletPolicyChunks(chunks)
			if err != nil {
				t.Fatal(err)
			}
			at, ok := complexAddressSource(chunks, keys)
			if !ok {
				t.Fatal("the device derives no address for a kind-1 wallet Liana imports")
			}
			for i := 0; i < 3; i++ {
				for _, leg := range []struct {
					change bool
					want   string
				}{{false, c.LianaReceive[i]}, {true, c.LianaChange[i]}} {
					got, err := at(uint32(i), leg.change)
					if err != nil {
						t.Fatalf("index %d change=%v: %v", i, leg.change, err)
					}
					if got != leg.want {
						t.Fatalf("index %d change=%v:\n device %s\n liana  %s", i, leg.change, got, leg.want)
					}
				}
			}
			// THE SAME TREE AT KIND 0 IS A DIFFERENT WALLET (§8.3's last
			// sentence): the device must not show Liana's addresses for it.
			nums := lianaCaseChunks(t, c, md.UnspendableNums)
			_, nkeys, err := md.ExpandWalletPolicyChunks(nums)
			if err != nil {
				t.Fatal(err)
			}
			nat, ok := complexAddressSource(nums, nkeys)
			if !ok {
				t.Fatal("the kind-0 twin derives no address")
			}
			if a, _ := nat(0, false); a == c.LianaReceive[0] {
				t.Fatalf("kind 0 and kind 1 derive the same receive 0 %s", a)
			}
		})
	}
	// Five at descriptor-mnemonic cf35d61a. A count, not ">0".
	if accepted != 5 {
		t.Fatalf("%d accepted Liana cases, want 5", accepted)
	}
}

// TestAnUnderivableInternalKeyKindIsRefusedNotFallenBack is SPEC §7a.3 at the
// device: a kind this firmware has no arm for yields an error, never the NUMS
// point. No decoder yields such a kind today, so the kind is constructed.
//
// Mutation: adding `default: return address.NUMSInternalKey()` fails; so does
// grouping the Liana arm with NUMS (`case md.InternalKeyNUMS,
// md.InternalKeyLianaUnspendable:`), via the nil-liana row.
func TestAnUnderivableInternalKeyKindIsRefusedNotFallenBack(t *testing.T) {
	if _, err := taprootInternalKey(md.InternalKeyKind(3), 0, nil, nil, 0, false); err != errUnderivableInternalKey {
		t.Fatalf("an unknown kind derived (err %v)", err)
	}
	if _, err := taprootInternalKey(md.InternalKeyLianaUnspendable, 0, nil, nil, 0, false); err != errUnderivableInternalKey {
		t.Fatalf("a kind-1 key with no computed Liana key derived (err %v)", err)
	}
}

// TestTemplatePlusKeyCardsDerivesTheLianaWallet is R0 I1's route: the
// composer's usual multi-party output is a key-less TEMPLATE plate plus one mk1
// key card per cosigner, and an operator proves that wallet by bringing all of
// them to Wallet Policy. The consent there must show Rust md 0.19.0's receive 0
// for the kind-1 wallet, exactly as it does for the kind-0 twin and for a
// tr wallet with a real key path.
//
// Mutation: lianaInternalKey building its map from the md1's OWN keys
// (md.ExpandWalletPolicyChunks(collected), which on a template carry no xpub)
// instead of `keys` -- the TLV read the first draft did -- fails the kind-1 row.
func TestTemplatePlusKeyCardsDerivesTheLianaWallet(t *testing.T) {
	for _, vec := range []string{"keyed_tr_liana_kofn_recovery", "keyed_compose_preset_kofn_recovery", "keyed_tr_with_leaf"} {
		t.Run(vec, func(t *testing.T) {
			tmpl, cards, want := seatFixtureFor(t, vec)
			if _, keys, err := md.ExpandWalletPolicyChunks(tmpl); err != nil || allSlotsHaveXpub(keys) {
				t.Fatalf("the fixture's md1 is not a key-less template (err %v)", err)
			}
			lines, err := walletPolicyConsentLines(tmpl, cards)
			if err != nil {
				t.Fatalf("consent refused: %v", err)
			}
			if joined := strings.Join(lines, "\n"); !strings.Contains(joined, want) {
				t.Fatalf("the consent does not show Rust's receive 0 %s:\n%s", want, joined)
			}
		})
	}
}
