package sysw

import "testing"

// ═══ THE TWO PARITY BREAKS REVIEW-S2-P3-r1 FOUND, AS TESTS ══════════════════
//
// Both were periphery of the arm rather than its predicate: the §4.7 conjunct
// port and the §4.5 narrowing agreed with the host on all 81 of the reviewer's
// constructed boundary cases. What diverged was WHICH STRING each check runs
// over -- C1 the version scan's scope, C2 the whitespace normalisation -- and
// both are constructible single-line inputs, which is what a record is.
//
// THE VECTOR FILE IS FROZEN. Invariant 1 gives S2 exactly one regeneration and
// it has landed, so neither finding can become a corpus row; and the file's one
// JSON row is multi-line, which is the blind spot C1 lived in. So these are
// unit tests, and each carries the HOST's MEASURED verdict in its comment so
// the cross-language pair is explicit rather than implied. Every verdict below
// was taken from `me sysw pack --no-passphrase --as descriptor --in <the exact
// bytes>` against the S2 `me` (rc 0 = admit, rc 3 = refuse).

// canonMultipath is `formats-happy/bip380-sortedmulti-multipath`'s canonical:
// three `xpub` keys, and `host_admits: true` in the vector file.
const canonMultipath = "wsh(sortedmulti(2," +
	"[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/*," +
	"[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/*," +
	"[c5d87297/48h/0h/0h/2h]xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*))#ud8uyjz3"

// ─── C1: a JSON label is a TITLE, and the host never parses it ───────────────
//
// Branch 3 copies `label` into `desc.Title` without parsing it
// (`nonstandard/parse.go:44-55`). A label is arbitrary operator text, and `"`,
// `{`, `}`, `:` and `,` are all outside the base58 alphabet -- so a label is its
// own maximal base58 run, and a whole-record version scan refused records whose
// every KEY is an `xpub`. Naming a wallet after an extended key is a legal thing
// to do.

// The reviewer's exact counterexample.
//
// MEASURED HOST VERDICT: rc 0, ADMIT -- `me` packs the canonical descriptor.
func TestJSONLabelIsNotKeyMaterial(t *testing.T) {
	rec := `{"label":"` + skYL + `","descriptor":"` + canonMultipath + `"}`
	if got := Classify(rec); got != ClassDescriptor {
		t.Errorf("Classify(json with a ypub LABEL) = %v, want ClassDescriptor -- "+
			"`me` admits this record (measured rc 0), because the cascade hands "+
			"parse_extended_key the `descriptor` field alone", got)
	}
}

// The sharper case, and the reason the fix scopes the SCAN rather than
// identifying keys after the fact: here the label is the `ypub` spelling of the
// very key the descriptor carries, so a remedy that matched each base58 run's
// key MATERIAL against the parsed keys would still refuse it. The host does not
// look at the label at all, so neither may this.
//
// MEASURED HOST VERDICT: rc 0, ADMIT.
func TestJSONLabelHoldingTheYpubTwinOfItsOwnKeyIsStillNotKeyMaterial(t *testing.T) {
	rec := `{"label":"` + skYL + `","descriptor":"wpkh(` + skXP + `/<0;1>/*)"}`
	if got := Classify(rec); got != ClassDescriptor {
		t.Errorf("Classify(json whose label is the ypub twin of its own key) = %v, "+
			"want ClassDescriptor", got)
	}
}

// The refusing direction, so the scoping is known to have narrowed the scan and
// not disabled it: a `ypub` smuggled as an actual KEY is still refused.
//
// MEASURED HOST VERDICT: rc 3, REFUSE -- "`me` admits exactly `xpub`, `tpub`,
// `zpub`, `Ypub`, `Zpub`. This key is `ypub`, whose equivalent is `xpub`: …".
func TestJSONDescriptorFieldCarryingAYpubKeyClassifiesUnknown(t *testing.T) {
	rec := `{"label":"my wallet","descriptor":"sh(wpkh([4bbaa801/49h/0h/0h]` +
		skYL + `/<0;1>/*))"}`
	if got := Classify(rec); got != ClassUnknown {
		t.Errorf("Classify(json whose DESCRIPTOR carries a ypub) = %v, want "+
			"ClassUnknown -- `me` refuses it at rc 3", got)
	}
}

// The plain-text-label control. Without it the two admitting tests above could
// pass for the wrong reason -- by the arm simply admitting anything JSON-shaped.
//
// MEASURED HOST VERDICT: rc 0, ADMIT.
func TestJSONPlainLabelClassifiesAsADescriptor(t *testing.T) {
	rec := `{"label":"my wallet","descriptor":"` + canonMultipath + `"}`
	if got := Classify(rec); got != ClassDescriptor {
		t.Errorf("Classify(json with a plain label) = %v, want ClassDescriptor", got)
	}
}

// ─── C2: §4.6 is ASCII-only, and the arm answers for its own ─────────────────
//
// `classifyConstellation` trims with `strings.TrimSpace` (`unicode.IsSpace`);
// the host's `cascade::normalise` trims with `char::is_ascii_whitespace`. The
// two sets differ by U+000B and the whole Unicode Zs category, so edge padding
// drawn from the difference made the device classify `ClassDescriptor` on
// records `me` refuses -- the device-WIDER direction, which is the one that puts
// a wallet the host would not pack in front of an operator.
//
// The shared trim predates S2 and the md1/mk1 arms rely on it being Unicode, so
// the fix is inside the descriptor arm alone.

type edgePad struct {
	name string
	pad  string
}

// asciiEdge is the five characters BOTH sides trim. U+000B is deliberately
// absent: Rust's `is_ascii_whitespace` is SPACE, TAB, LF, FF, CR and nothing
// else.
var asciiEdge = []edgePad{
	{"U+0020 space", " "},
	{"U+0009 tab", "\t"},
	{"U+000C form-feed", "\f"},
	{"U+000D carriage-return", "\r"},
	{"U+000A newline", "\n"},
}

// nonASCIIEdge is every character measured to diverge: Go trims it, Rust does
// not, so the device saw a wallet where `me` saw a parse error.
var nonASCIIEdge = []edgePad{
	{"U+000B vertical-tab", "\v"},
	{"U+0085 next-line", ""},
	{"U+00A0 no-break-space", " "},
	{"U+2003 em-space", " "},
	{"U+3000 ideographic-space", "　"},
}

// eachPadding runs f over both positions and both input forms, which is the
// grid the divergence was measured on -- 2 positions x 2 forms x 5 characters.
func eachPadding(t *testing.T, pads []edgePad, f func(t *testing.T, what, rec string)) {
	t.Helper()
	for _, w := range pads {
		for _, body := range []edgePad{
			{"descriptor", canonMultipath},
			{"bare key", skXP},
		} {
			f(t, "leading "+w.name+", "+body.name, w.pad+body.pad)
			f(t, "trailing "+w.name+", "+body.name, body.pad+w.pad)
		}
	}
}

// The control set, and it is what stops C2's fix from being a blanket refusal of
// padded records. §4.6 is NORMATIVE that the host trims these, and the shipped
// corpus row `whitespace/leading-space-bip380` is `host_admits: true` on exactly
// this basis.
//
// MEASURED HOST VERDICT: rc 0, ADMIT, on all 20 cases.
func TestASCIIEdgeWhitespaceStillClassifies(t *testing.T) {
	eachPadding(t, asciiEdge, func(t *testing.T, what, rec string) {
		if got := Classify(rec); got != ClassDescriptor {
			t.Errorf("%s: Classify = %v, want ClassDescriptor -- §4.6 trims ASCII "+
				"whitespace off both ends and the host admits the result", what, got)
		}
	})
}

// The finding itself: 20 measured divergences, every one device-wider.
//
// MEASURED HOST VERDICT: rc 3, REFUSE, on all 20 cases -- the padding survives
// `normalise`, so the cascade sees a script name (or a key) with a stray
// character in it and no branch parses.
func TestNonASCIIEdgeWhitespaceClassifiesUnknown(t *testing.T) {
	eachPadding(t, nonASCIIEdge, func(t *testing.T, what, rec string) {
		if got := Classify(rec); got != ClassUnknown {
			t.Errorf("%s: Classify = %v, want ClassUnknown -- `me` refuses it at rc 3, "+
				"and a device WIDER than the host here offers a wallet the host would "+
				"not pack", what, got)
		}
	})
}

// The other half of §4.6, so the guard pins the whole normalisation and not only
// its trim: the host replaces an INTERIOR CRLF with LF before parsing and the
// device's trim does not touch it, so the two are reading different strings.
// Neither admits the result, and asserting it is what keeps that true.
func TestInteriorCRLFClassifiesUnknown(t *testing.T) {
	rec := "wsh(sortedmulti(2,\r\n" + skXP + "))"
	if got := Classify(rec); got != ClassUnknown {
		t.Errorf("Classify(interior CRLF) = %v, want ClassUnknown", got)
	}
}

// Branch 1 has the same shape as branch 3 and is scoped the same way: a
// BlueWallet file's `Name:` value is a TITLE, and only the values of headers
// the parser does NOT recognise (the `fingerprint: xpub` pairs) ever reach
// ParseExtendedKey.
//
// This input is MULTI-LINE, so it can never be a sysw record -- the public
// section is split on LF. It is asserted anyway because the parity rule is
// stated over the predicate and not over reachability, and because the fix
// that closes C1 is the same one line of scoping for both branches: with the
// branch-1 arm removed, this classifies ClassUnknown while the host admits it
// (measured, both ways).
//
// The fixture is `formats-happy/bluewallet-sh-fixture` from the vector file,
// with its `Name:` value replaced by a `ypub`.
//
// MEASURED HOST VERDICT: rc 0, ADMIT -- with the original title and with the
// ypub title alike.
func TestBlueWalletNameHoldingAnExtendedKeyIsNotKeyMaterial(t *testing.T) {
	const tail = "Policy: 2 of 3\n" +
		"Derivation: m/48'/0'/0'/2'\n" +
		"Format: P2WSH\n" +
		"\n5A0804E3: xpub6F148LnjUhGrHfEN6Pa8VkwF8L6FJqYALxAkuHfacfVhMLVY4MRuUVMxr9pguAv67DHx1YFxqoKN8s4QfZtD9sR2xRCffTqi9E8FiFLAYk8\n" +
		"\nDD4FADEE: xpub6DnediUuY8Pcc6Fej8Yt2ZntPCyFdpbHBkNV7EawesRMbc6i9MKKMhKEv4JMMzwDJckaV4czBvNdc6ikwLiZqdUqMd5ZKQGYaQT4cXMeVjf\n" +
		"\n9BACD5C0: xpub6EefrCrMAduhNwnsHb3dAs8DYZSw4f63WyR6DaEByUHjwvPDdhczj15FyBBG4tbEJtf4vRKTv1ng5SPPnWv1Pve1f15EJfiBY5oYDN6VLEC\n"
	// The control first: the fixture as shipped classifies.
	if got := Classify("Name: sh\n" + tail); got != ClassDescriptor {
		t.Fatalf("the shipped BlueWallet fixture no longer classifies: %v", got)
	}
	if got := Classify("Name: " + skYL + "\n" + tail); got != ClassDescriptor {
		t.Errorf("Classify(BlueWallet whose Name is a ypub) = %v, want ClassDescriptor "+
			"-- `me` admits it (measured rc 0); the title is not key material", got)
	}
}
