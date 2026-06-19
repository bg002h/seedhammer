package md

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// singleStringVectorNames are the MANIFEST vectors whose .phrase.txt is a single
// md1 string (used for encodePayload byte-parity AND encodeMD1String string
// parity). The force-chunked wsh_multi_chunked is EXCLUDED (R0-M3): its
// .phrase.txt is a chunk-format string covered by the chunked round-trip tests.
var singleStringVectorNames = []string{
	"wpkh_basic",
	"pkh_basic",
	"wsh_multi_2of2",
	"wsh_multi_2of3",
	"wsh_sortedmulti",
	"tr_keyonly",
	"sh_wsh_multi",
	"wsh_divergent_paths",
	"wsh_with_fingerprints",
}

// byteParityVectorNames are all vectors with a .bytes.hex golden equal to the
// pre-chunk encodePayload output — the single-string set PLUS the force-chunked
// wsh_multi_chunked (whose .bytes.hex is the pre-chunk payload, R0-M3).
var byteParityVectorNames = append(append([]string(nil), singleStringVectorNames...), "wsh_multi_chunked")

func vectorPath(name, ext string) string {
	return filepath.Join("testdata", "vectors", name+"."+ext)
}

// readFileBytes decodes a vector's .bytes.hex into raw bytes without a *testing.T
// (used by fuzz seed corpora, which run before any sub-test).
func readFileBytes(name string) ([]byte, error) {
	raw, err := os.ReadFile(vectorPath(name, "bytes.hex"))
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(strings.TrimSpace(string(raw)))
}

func loadBytesHex(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "bytes.hex"))
	if err != nil {
		t.Fatalf("read %s.bytes.hex: %v", name, err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("decode %s.bytes.hex: %v", name, err)
	}
	return b
}

// loadPhrase returns the single md1 string from <name>.phrase.txt. For the
// force-chunked vector the file has a leading "chunk-set-id:" header line then
// the chunk string; this helper returns the LAST non-empty line (the actual md1
// string). Single-string vectors have exactly one line.
func loadPhrase(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "phrase.txt"))
	if err != nil {
		t.Fatalf("read %s.phrase.txt: %v", name, err)
	}
	var last string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "chunk-set-id:") {
			continue
		}
		last = line
	}
	if last == "" {
		t.Fatalf("%s.phrase.txt: no md1 string found", name)
	}
	return last
}

// ─── .descriptor.json → *descriptor loader (R0-M4) ───────────────────────────
//
// The Go AST uses interface-bodied nodes, so this is a custom shim, not a
// one-line json.Unmarshal. encoding/json is TinyGo-safe in _test.go.

type jsonDescriptor struct {
	N           uint8           `json:"n"`
	PathDecl    jsonPathDecl    `json:"path_decl"`
	UseSitePath jsonUseSitePath `json:"use_site_path"`
	Tree        jsonNode        `json:"tree"`
	TLV         jsonTLV         `json:"tlv"`
}

type jsonPathDecl struct {
	Tag  string          `json:"tag"`  // "Shared" | "Divergent"
	Data json.RawMessage `json:"data"` // "m"/path string when Shared; [path,...] when Divergent
}

type jsonUseSitePath struct {
	Multipath        []jsonAlt `json:"multipath"` // null → bare star
	WildcardHardened bool      `json:"wildcard_hardened"`
}

type jsonAlt struct {
	Hardened bool   `json:"hardened"`
	Value    uint32 `json:"value"`
}

type jsonNode struct {
	Tag  string   `json:"tag"`
	Body jsonBody `json:"body"`
}

type jsonBody struct {
	Kind string          `json:"kind"` // KeyArg|Children|Variable|MultiKeys|Tr|Hash256Body|Hash160Body|Timelock|Empty
	Data json.RawMessage `json:"data"`
}

type jsonTLV struct {
	UseSitePathOverrides []json.RawMessage `json:"use_site_path_overrides"`
	Fingerprints         []json.RawMessage `json:"fingerprints"`
	Pubkeys              []json.RawMessage `json:"pubkeys"`
	OriginPathOverrides  []json.RawMessage `json:"origin_path_overrides"`
	Unknown              []json.RawMessage `json:"unknown"`
}

var jsonTagTable = map[string]tag{
	"Wpkh": tagWpkh, "Tr": tagTr, "Wsh": tagWsh, "Sh": tagSh, "Pkh": tagPkh,
	"TapTree": tagTapTree, "Multi": tagMulti, "SortedMulti": tagSortedMulti,
	"MultiA": tagMultiA, "SortedMultiA": tagSortedMultiA, "PkK": tagPkK, "PkH": tagPkH,
	"Check": tagCheck, "Verify": tagVerify, "Swap": tagSwap, "Alt": tagAlt,
	"DupIf": tagDupIf, "NonZero": tagNonZero, "ZeroNotEqual": tagZeroNotEqual,
	"AndV": tagAndV, "AndB": tagAndB, "AndOr": tagAndOr, "OrB": tagOrB,
	"OrC": tagOrC, "OrD": tagOrD, "OrI": tagOrI, "Thresh": tagThresh,
	"After": tagAfter, "Older": tagOlder, "Sha256": tagSha256, "Hash160": tagHash160,
	"Hash256": tagHash256, "Ripemd160": tagRipemd160, "RawPkH": tagRawPkH,
	"False": tagFalse, "True": tagTrue,
}

func loadDescriptor(t *testing.T, name string) *descriptor {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "descriptor.json"))
	if err != nil {
		t.Fatalf("read %s.descriptor.json: %v", name, err)
	}
	var jd jsonDescriptor
	if err := json.Unmarshal(raw, &jd); err != nil {
		t.Fatalf("unmarshal %s.descriptor.json: %v", name, err)
	}
	d := &descriptor{n: jd.N}
	d.pathDecl = buildPathDecl(t, jd.N, jd.PathDecl)
	d.useSite = buildUseSite(jd.UseSitePath)
	d.tree = buildNode(t, jd.Tree)
	d.tlv = buildTLV(t, jd.TLV)
	return d
}

// parsePathString parses a BIP-32 path like "m/84'/0'/0'" (or "m") into an
// originPath. Empty/"m" → no components.
func parsePathString(t *testing.T, s string) originPath {
	t.Helper()
	s = strings.TrimSpace(s)
	if s == "" || s == "m" {
		return originPath{}
	}
	parts := strings.Split(s, "/")
	var comps []pathComponent
	for _, p := range parts {
		if p == "m" || p == "" {
			continue
		}
		hardened := false
		if strings.HasSuffix(p, "'") || strings.HasSuffix(p, "h") || strings.HasSuffix(p, "H") {
			hardened = true
			p = p[:len(p)-1]
		}
		var v uint32
		for _, c := range p {
			if c < '0' || c > '9' {
				t.Fatalf("bad path component %q in %q", p, s)
			}
			v = v*10 + uint32(c-'0')
		}
		comps = append(comps, pathComponent{hardened: hardened, value: v})
	}
	return originPath{components: comps}
}

func buildPathDecl(t *testing.T, n uint8, jp jsonPathDecl) pathDecl {
	t.Helper()
	switch jp.Tag {
	case "Shared":
		var s string
		if err := json.Unmarshal(jp.Data, &s); err != nil {
			t.Fatalf("path_decl Shared data: %v", err)
		}
		p := parsePathString(t, s)
		return pathDecl{n: n, shared: &p}
	case "Divergent":
		var arr []string
		if err := json.Unmarshal(jp.Data, &arr); err != nil {
			t.Fatalf("path_decl Divergent data: %v", err)
		}
		paths := make([]originPath, len(arr))
		for i, s := range arr {
			paths[i] = parsePathString(t, s)
		}
		return pathDecl{n: n, divergent: paths}
	default:
		t.Fatalf("unknown path_decl tag %q", jp.Tag)
		return pathDecl{}
	}
}

func buildUseSite(ju jsonUseSitePath) useSitePath {
	out := useSitePath{wildcardHardened: ju.WildcardHardened}
	if ju.Multipath != nil {
		out.hasMultipath = true
		out.multipath = make([]alternative, len(ju.Multipath))
		for i, a := range ju.Multipath {
			out.multipath[i] = alternative{hardened: a.Hardened, value: a.Value}
		}
	}
	return out
}

func buildNode(t *testing.T, jn jsonNode) node {
	t.Helper()
	tg, ok := jsonTagTable[jn.Tag]
	if !ok {
		t.Fatalf("unknown tag %q", jn.Tag)
	}
	var b body
	switch jn.Body.Kind {
	case "KeyArg":
		var d struct {
			Index uint8 `json:"index"`
		}
		mustJSON(t, jn.Body.Data, &d)
		b = keyArgBody{index: d.Index}
	case "Children":
		var arr []jsonNode
		mustJSON(t, jn.Body.Data, &arr)
		children := make([]node, len(arr))
		for i, c := range arr {
			children[i] = buildNode(t, c)
		}
		b = childrenBody{children: children}
	case "Variable":
		var d struct {
			K        uint8      `json:"k"`
			Children []jsonNode `json:"children"`
		}
		mustJSON(t, jn.Body.Data, &d)
		children := make([]node, len(d.Children))
		for i, c := range d.Children {
			children[i] = buildNode(t, c)
		}
		b = variableBody{k: d.K, children: children}
	case "MultiKeys":
		var d struct {
			K       uint8   `json:"k"`
			Indices []uint8 `json:"indices"`
		}
		mustJSON(t, jn.Body.Data, &d)
		b = multiKeysBody{k: d.K, indices: d.Indices}
	case "Tr":
		var d struct {
			IsNums   bool      `json:"is_nums"`
			KeyIndex uint8     `json:"key_index"`
			Tree     *jsonNode `json:"tree"`
		}
		mustJSON(t, jn.Body.Data, &d)
		tb := trBody{isNums: d.IsNums, keyIndex: d.KeyIndex}
		if d.Tree != nil {
			sub := buildNode(t, *d.Tree)
			tb.tree = &sub
		}
		b = tb
	case "Hash256Body":
		var arr []byte
		mustJSON(t, jn.Body.Data, &arr)
		var h hash256Body
		copy(h[:], arr)
		b = h
	case "Hash160Body":
		var arr []byte
		mustJSON(t, jn.Body.Data, &arr)
		var h hash160Body
		copy(h[:], arr)
		b = h
	case "Timelock":
		var v uint32
		mustJSON(t, jn.Body.Data, &v)
		b = timelockBody(v)
	case "Empty":
		b = emptyBody{}
	default:
		t.Fatalf("unknown body kind %q", jn.Body.Kind)
	}
	return node{tag: tg, body: b}
}

func buildTLV(t *testing.T, jt jsonTLV) tlvSection {
	t.Helper()
	var s tlvSection
	if jt.UseSitePathOverrides != nil {
		s.useSitePresent = true
		for _, e := range jt.UseSitePathOverrides {
			var pair [2]json.RawMessage
			mustJSON(t, e, &pair)
			var idx uint8
			mustJSON(t, pair[0], &idx)
			var ju jsonUseSitePath
			mustJSON(t, pair[1], &ju)
			s.useSiteOverrides = append(s.useSiteOverrides, idxUseSite{idx: idx, path: buildUseSite(ju)})
		}
	}
	if jt.Fingerprints != nil {
		s.fpPresent = true
		for _, e := range jt.Fingerprints {
			var pair [2]json.RawMessage
			mustJSON(t, e, &pair)
			var idx uint8
			mustJSON(t, pair[0], &idx)
			var hexstr string
			mustJSON(t, pair[1], &hexstr)
			fb, err := hex.DecodeString(hexstr)
			if err != nil || len(fb) != 4 {
				t.Fatalf("bad fingerprint %q", hexstr)
			}
			var fp [4]byte
			copy(fp[:], fb)
			s.fingerprints = append(s.fingerprints, idxFP{idx: idx, fp: fp})
		}
	}
	if jt.Pubkeys != nil {
		s.pubPresent = true
		for _, e := range jt.Pubkeys {
			var pair [2]json.RawMessage
			mustJSON(t, e, &pair)
			var idx uint8
			mustJSON(t, pair[0], &idx)
			var arr []byte
			mustJSON(t, pair[1], &arr)
			var xpub [65]byte
			copy(xpub[:], arr)
			s.pubkeys = append(s.pubkeys, idxPub{idx: idx, xpub: xpub})
		}
	}
	if jt.OriginPathOverrides != nil {
		s.originPresent = true
		for _, e := range jt.OriginPathOverrides {
			var pair [2]json.RawMessage
			mustJSON(t, e, &pair)
			var idx uint8
			mustJSON(t, pair[0], &idx)
			var pathStr string
			mustJSON(t, pair[1], &pathStr)
			s.originOverrides = append(s.originOverrides, idxOrigin{idx: idx, path: parsePathString(t, pathStr)})
		}
	}
	return s
}

func mustJSON(t *testing.T, raw json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("unmarshal %s: %v", string(raw), err)
	}
}
