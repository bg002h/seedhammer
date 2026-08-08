package seal

import (
	"errors"
	"strings"
	"testing"

	"seedhammer.com/codex32"
	"seedhammer.com/mk"
)

// §10.2.1's allow-list and §6.3's card-set decode. This is the task that stops
// a seed reaching steel in the clear.
//
// Every fixture below other than the forged ones is a real record out of
// testdata/vectors.json, and every chunk-set id asserted here was MEASURED
// against the fork's own md/mk packages, not copied from a doc comment.

const smuggledMD1 = "md1qqqsyqcyq5rqwzqfpg9scrgwpugpzysnzs23v9ccrydpk8qarc0sdmjzeptm5fdk0"

// Two distinct NON-chunked md1 cards, from md/md_test.go's decode parity table
// (wsh_multi_2of2 and wsh_multi_2of3). Measured: ParseChunkHeader returns
// {Chunked:false ChunkSetID:0}, md.Decode succeeds, md.Reassemble fails with
// "wire version mismatch".
const (
	singleMD1a = "md1yppqqxppsg2vlumagltz27le"
	singleMD1b = "md1yzpqqxppsgsc8dua4tu0kekyl"
)

// THE lowercase check, and it runs BEFORE classification and binds BOTH
// sections. Not cosmetic: measured against the fork, ValidMD returns TRUE for a
// fully uppercased md1 — by design, and the device's own keyboard-entry path
// emits uppercase. Without this check an uppercase record is admitted, engraved
// verbatim, and displayed with a §6.6 hash the operator's recorded value cannot
// match on an untampered payload — teaching them that mismatches are normal,
// which disarms the single control §6.6 provides.
func TestRefusesAnUppercaseRecord(t *testing.T) {
	d := vectorNamed(t, "D")
	upperMD := strings.ToUpper(d.Public[2])
	if !codex32.ValidMD(upperMD) {
		t.Fatal("premise broken: an uppercased md1 must still pass ValidMD, " +
			"or this test is not testing what it says")
	}
	upperMS := strings.ToUpper(d.Secret[0])

	for _, c := range []struct {
		name    string
		section Section
		records []string
	}{
		{"public", SectionPublic, replaceAt(d.Public, 2, upperMD)},
		{"encrypted", SectionEncrypted, []string{upperMS}},
		{"encrypted md1", SectionEncrypted, []string{upperMD}},
	} {
		got, err := AdmitSection(bs(c.records), c.section)
		if !errors.Is(err, ErrNotLowercase) {
			t.Errorf("%s: uppercase record must be refused, got %v", c.name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s: %d records admitted on a rejected payload", c.name, len(got))
		}
	}
}

// The irreversible branch. Platform.LockBoot (cmd/controller/platform_sh2.go:545)
// does writeOTPValues -> otp.EnableSecureBoot -> machine.CPUReset, reached from
// gui/gui.go:1672 with the "command: " prefix (gui/scan.go:57) as the only gate.
//
// The command sits at INDEX 2 of a 6-record section, not index 0. At index 0
// the test passes under a loop that validates records[0] and trusts the rest.
func TestPublicSectionRefusesDebugCommand(t *testing.T) {
	d := vectorNamed(t, "D")
	recs := insertAt(d.Public, 2, "command: lock-boot")
	if len(recs) != 6 {
		t.Fatalf("the section must be 6 records for this to bind, got %d", len(recs))
	}
	if Classify([]byte(recs[2])) != ClassDebugCommand {
		t.Fatalf("record 2 must classify as a debug command, got %v", Classify([]byte(recs[2])))
	}
	got, err := AdmitSection(bs(recs), SectionPublic)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("a debug command must reject the payload, got %v", err)
	}
	// Phase A's stand-in for §11.2's "nothing was engraved": no record is
	// handed back at all. A deny-list, or a loop that checks record 0 and
	// trusts the rest, would hand back records 0-2 before meeting the command.
	if len(got) != 0 {
		t.Errorf("%d records admitted on a rejected payload — nothing may be engraved", len(got))
	}
	// The encrypted section is not a loophole either.
	if _, err := AdmitSection(bs([]string{"command: lock-boot"}), SectionEncrypted); !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("a debug command must be refused in the encrypted section too, got %v", err)
	}
}

// The other two extra classifier branches. An allow-list refuses them; a
// deny-list would silently admit whatever branch Scan grows next.
func TestPublicSectionRefusesAddressAndDescriptor(t *testing.T) {
	const (
		addr = "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
		desc = "wpkh([00000000/84h/0h/0h]xpub6CatWdiZiodmUeTDp8LT5or8nmbKNcuyvz7WyksVFkKB4RHwCD3XyuvPEbvqAQY3rAPshWcMLoP2fMFMKHPJ4ZeZXYVUhLv1VMrjPC7PW6V/<0;1>/*)"
	)
	if got := Classify([]byte(addr)); got != ClassAddress {
		t.Errorf("Classify([]byte(address)) = %v, want %v", got, ClassAddress)
	}
	if got := Classify([]byte(desc)); got != ClassDescriptor {
		t.Errorf("Classify([]byte(descriptor)) = %v, want %v", got, ClassDescriptor)
	}
	// Bind the allow-list branch directly. Neither classification is permitted
	// in EITHER section, which is what makes this an allow-list: a deny-list
	// would have to enumerate them.
	for _, c := range []Classification{ClassDescriptor, ClassAddress, ClassDebugCommand, ClassUnknown} {
		for _, sec := range []Section{SectionPublic, SectionEncrypted} {
			if permitted(sec, c) {
				t.Errorf("%v must not be permitted in the %s section", c, sec)
			}
		}
	}

	d := vectorNamed(t, "D")
	// An address is all-lowercase, so it reaches the allow-list and is
	// refused there — the end-to-end path this test exists for.
	for _, sec := range []Section{SectionPublic, SectionEncrypted} {
		got, err := AdmitSection(bs(insertAt(d.Public, 2, addr)), sec)
		if !errors.Is(err, ErrRecordNotPermitted) {
			t.Errorf("%s section, address: got %v", sec, err)
		}
		if len(got) != 0 {
			t.Errorf("%d records admitted on a rejected payload", len(got))
		}
	}
	// A real output descriptor carries an xpub, which is mixed case, so §6.4's
	// lowercase rule refuses it one pass EARLIER — measured, not assumed:
	// "record is not lowercase: record 2, byte 30". The payload is still
	// rejected with nothing admitted, which is the guarantee §11.2 asks for;
	// the allow-list branch itself is bound by the permitted() table above.
	for _, sec := range []Section{SectionPublic, SectionEncrypted} {
		got, err := AdmitSection(bs(insertAt(d.Public, 2, desc)), sec)
		if err == nil {
			t.Errorf("%s section: an output descriptor must reject the payload", sec)
		}
		if !errors.Is(err, ErrNotLowercase) && !errors.Is(err, ErrRecordNotPermitted) {
			t.Errorf("%s section, descriptor: unexpected error %v", sec, err)
		}
		if len(got) != 0 {
			t.Errorf("%d records admitted on a rejected payload", len(got))
		}
	}
}

// The single most important negative case in the suite: it is what stops a seed
// reaching steel unencrypted.
func TestPublicSectionRefusesASecret(t *testing.T) {
	d := vectorNamed(t, "D")
	ms1 := d.Secret[0]
	if got := Classify([]byte(ms1)); got != ClassCodex32Secret {
		t.Fatalf("Classify([]byte(ms1)) = %v, want %v", got, ClassCodex32Secret)
	}
	got, err := AdmitSection(bs(insertAt(d.Public, 2, ms1)), SectionPublic)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("an ms1 in the public section must reject the payload, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted — nothing may be engraved", len(got))
	}
	// ...and the same record IS admitted in the encrypted section (§12 item 6:
	// ms1 is admitted encrypted and refused in the clear, deliberately
	// separable).
	if _, err := AdmitSection(bs([]string{ms1}), SectionEncrypted); err != nil {
		t.Errorf("ms1 must be admitted in the encrypted section: %v", err)
	}
	// So must a BIP-39 mnemonic, and it must NOT be admitted in public.
	mn := vectorNamed(t, "A").Secret[0]
	if got := Classify([]byte(mn)); got != ClassMnemonic {
		t.Fatalf("Classify([]byte(mnemonic)) = %v, want %v", got, ClassMnemonic)
	}
	if _, err := AdmitSection(bs([]string{mn}), SectionEncrypted); err != nil {
		t.Errorf("a BIP-39 mnemonic must be admitted in the encrypted section: %v", err)
	}
	if _, err := AdmitSection(bs([]string{mn}), SectionPublic); !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("a BIP-39 mnemonic in the public section must be refused, got %v", err)
	}
}

// §6.3: DECODE is per CARD SET, not per record, because records are CHUNKS. A
// per-record decode would reject every legitimate payload.
func TestDecodesACompleteCardSet(t *testing.T) {
	for _, name := range []string{"D", "E", "G"} {
		v := vectorNamed(t, name)
		got, err := AdmitSection(bs(v.Public), SectionPublic)
		if err != nil {
			t.Errorf("%s: the full card set must decode: %v", name, err)
		}
		if len(got) != len(v.Public) {
			t.Errorf("%s: admitted %d of %d records", name, len(got), len(v.Public))
		}
		for i, a := range got {
			if a.Class != ClassMDMK {
				t.Errorf("%s record %d classified %v, want %v", name, i, a.Class, ClassMDMK)
			}
			if string(a.Record) != v.Public[i] {
				t.Errorf("%s record %d was altered", name, i)
			}
		}
	}
}

func TestRefusesAnIncompleteCardSet(t *testing.T) {
	d := vectorNamed(t, "D")
	// D's public section is mk1 chunks 0-1 (csid 92715) then md1 chunks 0-2
	// (csid 398802) — measured.
	for _, c := range []struct {
		name    string
		records []string
	}{
		{"one md1 chunk of three", d.Public[2:3]},
		{"two md1 chunks of three", d.Public[2:4]},
		{"one mk1 chunk of two", d.Public[0:1]},
	} {
		got, err := AdmitSection(bs(c.records), SectionPublic)
		if !errors.Is(err, ErrUndecodableCardSet) {
			t.Errorf("%s must be refused, got %v", c.name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s: %d records admitted", c.name, len(got))
		}
	}
}

// The smuggling case. ValidMD/ValidMK are pure BCH verifiers that never decode,
// and the fork ships the checksum generator (MDChecksumSymbols, AssembleMD1), so
// arbitrary bytes wrap into a record that classifies as mdmkText. Without the
// decode step, seed entropy rides in the cleartext section where `picotool save`
// reaches it with no passphrase and mdmkFlow engraves it verbatim.
func TestRefusesBCHValidButUndecodable(t *testing.T) {
	if !codex32.ValidMD(smuggledMD1) {
		t.Fatal("premise broken: the BCH layer must ACCEPT this record — that is the point")
	}
	if got := Classify([]byte(smuggledMD1)); got != ClassMDMK {
		t.Fatalf("Classify([]byte(smuggled)) = %v, want %v — the allow-list alone lets it through", got, ClassMDMK)
	}
	got, err := AdmitSection(bs([]string{smuggledMD1}), SectionPublic)
	if !errors.Is(err, ErrUndecodableCardSet) {
		t.Errorf("the decode step must reject it, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted", len(got))
	}
}

// THE grouping test, and vector G is the only payload that can carry it. D and
// E hold one card per HRP and F is pub_len = 0, so an implementation grouping
// by HRP alone passes all of them. G's public section is FOUR cards: one md1
// chunked six ways plus three mk1 cards of two chunks each — one per cosigner.
//
// All chunk-set ids below were measured with md.ParseChunkHeader /
// mk.ParseHeader against the vector's own records.
func TestGroupsByHRPAndChunkSetID(t *testing.T) {
	g := vectorNamed(t, "G")
	if len(g.Public) != 12 {
		t.Fatalf("vector G's public section must be 12 records, got %d", len(g.Public))
	}
	keys, groups, err := groupCards(g.Public)
	if err != nil {
		t.Fatalf("groupCards: %v", err)
	}
	want := []struct {
		key  groupKey
		size int
	}{
		{groupKey{hrp: 'k', chunked: true, csid: 153721}, 2},
		{groupKey{hrp: 'k', chunked: true, csid: 153720}, 2},
		{groupKey{hrp: 'k', chunked: true, csid: 153723}, 2},
		{groupKey{hrp: 'd', chunked: true, csid: 841149}, 6},
	}
	if len(keys) != len(want) {
		t.Fatalf("grouped into %d cards, want %d — HRP-alone grouping gives 2", len(keys), len(want))
	}
	for _, w := range want {
		set, ok := groups[w.key]
		if !ok {
			t.Errorf("missing card %+v", w.key)
			continue
		}
		if len(set) != w.size {
			t.Errorf("card %+v has %d chunks, want %d", w.key, len(set), w.size)
		}
	}
	// ...and every group reassembles and decodes, with no leftovers.
	if _, err := AdmitSection(bs(g.Public), SectionPublic); err != nil {
		t.Errorf("vector G's public section must be admitted: %v", err)
	}

	// Why this test exists, made checkable rather than asserted in a comment:
	// grouping by HRP alone lumps all six mk1 records into one set and rejects
	// EVERY multisig wallet.
	var allMK []string
	for _, r := range g.Public {
		if codex32.ValidMK(r) {
			allMK = append(allMK, r)
		}
	}
	if len(allMK) != 6 {
		t.Fatalf("expected 6 mk1 records in G, got %d", len(allMK))
	}
	if _, err := mk.Decode(allMK); err == nil {
		t.Error("premise broken: HRP-alone grouping must fail, or G proves nothing")
	} else {
		t.Logf("HRP-alone grouping would give: %v", err)
	}
}

// Two single-string md1 cards must NOT collide into one group, and each must
// route to md.Decode. Vectors D, E and G are all chunked, so without this the
// mutant "route every group to md.Reassemble, drop the md.Decode arm" survives
// the entire suite.
//
// The collision risk is real and specific: ParseChunkHeader returns
// ChunkSetID 0 for a non-chunked record, and 0 is ALSO a legal value for a
// chunked one — so keying on csid conflates the two in both directions.
func TestDecodesTwoDistinctNonChunkedCards(t *testing.T) {
	recs := []string{singleMD1a, singleMD1b}
	keys, groups, err := groupCards(recs)
	if err != nil {
		t.Fatalf("groupCards: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("two distinct single-string cards grouped into %d cards, want 2", len(keys))
	}
	for _, k := range keys {
		if k.chunked {
			t.Errorf("card %+v must not be marked chunked", k)
		}
		if len(groups[k]) != 1 {
			t.Errorf("card %+v holds %d records, want 1", k, len(groups[k]))
		}
	}
	got, err := AdmitSection(bs(recs), SectionPublic)
	if err != nil {
		t.Errorf("two non-chunked md1 cards must decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("admitted %d records, want 2", len(got))
	}
	// A non-chunked card mixed in with a chunked set keeps its own group.
	d := vectorNamed(t, "D")
	if _, err := AdmitSection(bs(append([]string{singleMD1a}, d.Public...)), SectionPublic); err != nil {
		t.Errorf("a single-string card alongside D's chunked cards must decode: %v", err)
	}
}

// F-67's 93-symbol md1 codeword cap is load-bearing at THIS layer, not only
// inside codex32: past 93 symbols BCH(93,80,8) aliases and the checksum stops
// meaning what it claims, so md-codec rejects with StringSymbolCountOutOfRange
// and this port must agree.
func TestRefusesAnOverlongRecord(t *testing.T) {
	atCap := codex32.AssembleMD1(make([]byte, 80)) // 93 data symbols — legal
	over := codex32.AssembleMD1(make([]byte, 81))  // 94 data symbols — refused
	if got := Classify([]byte(atCap)); got != ClassMDMK {
		t.Errorf("a 93-symbol codeword must still classify as md1/mk1, got %v", got)
	}
	if got := Classify([]byte(over)); got != ClassUnknown {
		t.Errorf("a 94-symbol codeword must not classify as md1/mk1, got %v", got)
	}
	d := vectorNamed(t, "D")
	got, err := AdmitSection(bs(insertAt(d.Public, 2, over)), SectionPublic)
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("an over-long md1 must reject the payload, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted", len(got))
	}
}

// A record belonging to no complete group rejects the payload. Here D's five
// public records plus one chunk of G's six-chunk md1 card: the intruder forms a
// group of its own that can never complete.
func TestLeftoverRecordRejects(t *testing.T) {
	d := vectorNamed(t, "D")
	g := vectorNamed(t, "G")
	var intruder string
	for _, r := range g.Public {
		if codex32.ValidMD(r) {
			intruder = r
			break
		}
	}
	if intruder == "" {
		t.Fatal("no md1 record found in vector G")
	}
	recs := append(append([]string(nil), d.Public...), intruder)
	got, err := AdmitSection(bs(recs), SectionPublic)
	if !errors.Is(err, ErrUndecodableCardSet) {
		t.Errorf("a leftover record must reject the payload, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("%d records admitted", len(got))
	}
}

// The classifier mirrors gui/scan.go's branch ORDER, which is what makes this
// an allow-list rather than a re-implementation that can drift.
func TestClassifyMirrorsScanBranchOrder(t *testing.T) {
	d := vectorNamed(t, "D")
	for _, c := range []struct {
		in   string
		want Classification
	}{
		{"command: lock-boot", ClassDebugCommand},
		{vectorNamed(t, "A").Secret[0], ClassMnemonic},
		{d.Secret[0], ClassCodex32Secret},
		{d.Public[0], ClassMDMK}, // mk1
		{d.Public[2], ClassMDMK}, // md1
		{"", ClassUnknown},
		{"not a record at all", ClassUnknown},
	} {
		if got := Classify([]byte(c.in)); got != c.want {
			t.Errorf("Classify(%.24q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func replaceAt(src []string, i int, s string) []string {
	out := append([]string(nil), src...)
	out[i] = s
	return out
}

func insertAt(src []string, i int, s string) []string {
	out := append([]string(nil), src[:i]...)
	out = append(out, s)
	return append(out, src[i:]...)
}
