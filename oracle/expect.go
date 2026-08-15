package oracle

// The DERIVED expectation: what a walk's recorded inputs REQUIRE to have been
// engraved, computed by invoking the primary toolchain.
//
// # Why this file exists (F-170, F-171)
//
// Two holes, and they are the same hole from opposite ends.
//
// F-170: the walk asserted a plate COUNT — `census.strings.length === plates`,
// with `plates` a parameter defaulting to 6. A walk that engraved six WRONG
// strings was green. The plan's §3 preamble has forbidden that since it was
// written: "A walk's expected artifact census MUST derive from the recorded
// input tuple, never from what the walk produced… The script computes how many
// md1 chunks, mk1s and ms1s the inputs REQUIRE and fails when fewer arrive."
// Nothing implemented it.
//
// F-171: nothing in the tree invoked the pinned md/mk/ms at all, so the
// byte-identity gates S2 and S5 rest on had no mechanism — unimplemented, not
// merely unrun.
//
// Deriving the expectation is what closes both: the count falls out of the
// derivation (len of the result), and the strings are produced by the primary
// rather than copied from the device.
//
// # What the oracle chain covers, exactly
//
// Every step is a primary-toolchain invocation. Nothing here re-implements a
// derivation, and nothing is taken from the device:
//
//	seed words  --ms derive--> master fingerprint + account xpub
//	account xpub --mk encode--> the mk1 chunk(s)
//
// The `ms derive --template bip48-*` half did not exist until 2026-08-14: ms
// offered single-sig templates only and accepted no literal path, so seed ->
// multisig account xpub had NO oracle and this file could only have started
// from an xpub somebody typed in. That gap is why the templates were added
// upstream first (ms-cli 0.15.0) rather than worked around here.
//
// # The origin path is CHECKED, not merely recorded
//
// The recorded tuple names a per-slot origin. This code does not trust it: it
// derives the template's own path and REFUSES when the two disagree. A record
// whose origins drifted from the key material would otherwise describe a
// different wallet than the one engraved, and nothing would notice.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ExpectKind names the shape of a walk's artifact set. It is a machine key, not
// prose: the tuple's Template field is human text and cannot dispatch.
//
// Deliberately a closed set with no default. An unknown kind REFUSES rather
// than deriving nothing and reporting a vacuous match — "zero expected strings,
// zero mismatches" is the exact false-PASS this stage exists to remove.
type ExpectKind string

const (
	// KindCosignerCards: the walk engraves gathered cosigner mk1 cards
	// verbatim, one card per recorded seed, at that seed's recorded origin.
	// This is Trace A's shape and S0's record.
	KindCosignerCards ExpectKind = "cosigner-cards"
)

// KnownExpectKind reports whether DeriveExpected can actually derive this kind,
// and ArtifactKindFor names the artifact kind it produces.
//
// These exist so the UNTAGGED tests can ask the same question DeriveExpected
// asks, without a toolchain. Re-review finding I-1: a hand-authored expectation
// naming a kind nothing derives, carrying artifacts of a kind that kind never
// produces, passed the whole untagged suite green — because the only check that
// would have caught it lived behind the `oraclelive` build tag, which nothing
// routinely runs. A check that exists only where it is never run is the same
// defect as the skip this stage was built to remove.
//
// Keep this switch and DeriveExpected's in agreement: DeriveExpected now calls
// KnownExpectKind rather than repeating the comparison, so there is one source
// of truth and a new kind cannot be half-added.
func KnownExpectKind(k ExpectKind) bool {
	return ArtifactKindFor(k) != ""
}

// ArtifactKindFor returns the Artifact.Kind ("md1"/"mk1"/"ms1") that the given
// ExpectKind produces, or "" if the kind is not derivable. A kind that engraves
// several artifact kinds needs a richer answer than this; add it when a stage
// actually has that shape rather than guessing now.
func ArtifactKindFor(k ExpectKind) string {
	switch k {
	case KindCosignerCards:
		return "mk1"
	default:
		return ""
	}
}

// Expect is the part of the derivation that the input TUPLE cannot supply.
//
// Kept as small as it can be. Everything derivable from the tuple — origins,
// seeds, order — is derived from the tuple, because a field restated here is a
// field that can disagree with the record.
type Expect struct {
	Kind ExpectKind `json:"kind"`
	// PolicyIDStub is the 4-byte stub every cosigner card carries, hex. It is
	// an INPUT to mk1 encoding and is not recoverable from a seed, so it must
	// be stated.
	PolicyIDStub string `json:"policy_id_stub"`
}

// Bins are the resolved oracle binary paths.
//
// ABSOLUTE PATHS, always, and they come from pin resolution. Never invoke an
// oracle by bare name: on the maintainer's own machine `md` is a shell alias
// for `mkdir -p`, so a gate that shelled out through a shell would silently run
// coreutils and report whatever that did. exec.Command does not use a shell, so
// this is safe by construction — stated because the next person to write a
// convenience wrapper needs to know.
type Bins struct {
	MD, MK, MS string
}

// Artifact is one expected engraved string and the invocation that produced it.
//
// JSON-tagged because these are what a committed expectation holds
// (expectfile.go); the tags are the on-disk shape, so renaming a field is a
// schema change.
type Artifact struct {
	// Kind is "md1", "mk1" or "ms1".
	Kind string `json:"kind"`
	// Label names which input produced it, e.g. "payload:masterA (card A@0)".
	Label string `json:"label"`
	// String is the expected engraved string, byte for byte.
	String string `json:"string"`
	// Origin is the derivation path the oracle CONFIRMED for this artifact.
	Origin string `json:"origin,omitempty"`
	// Fingerprint is the master fingerprint the oracle derived from the seed.
	// Recorded because it binds the seed to the card independently of the xpub.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// DerivedSet is what one live derivation produced, plus enough about HOW to
// mint a provenance block that is measured rather than declared.
//
// Oracles and Args are collected by DeriveExpected as it invokes each binary —
// never from a table listing which oracles a kind "uses", because such a table
// is a claim that decays the first time a kind grows a step.
type DerivedSet struct {
	// Artifacts is the expected engraved set, in order.
	Artifacts []Artifact
	// Oracles names the binaries actually invoked, in first-use order.
	Oracles []string
	// Args records each distinct invocation FORM, with key material redacted.
	// A committed expectation carries these so a human can redo the derivation;
	// nothing parses them.
	//
	// Redacted deliberately: seed words on a recorded argv would put key
	// material in a committed file and in CI logs, which is the same refusal
	// SeedRef exists for.
	Args []string
}

// note records an oracle invocation on a DerivedSet, deduplicating both lists.
func (d *DerivedSet) note(oracle, argForm string) {
	if !slices.Contains(d.Oracles, oracle) {
		d.Oracles = append(d.Oracles, oracle)
	}
	if !slices.Contains(d.Args, argForm) {
		d.Args = append(d.Args, argForm)
	}
}

// Seed is one recorded seed's words, supplied alongside the tuple.
//
// Words are NOT in InputTuple by design — a gate record carrying seed words
// would be key material in a repo and in CI logs (see SeedRef). They live in
// the operator-authored inputs file, which is committed only because these
// particular masters are BIP-39's own published vectors.
type Seed struct {
	Label string
	Words string
}

// DeriveExpected computes the artifacts a walk's inputs require, in engrave
// order, by invoking the primary toolchain. It also reports which oracles it
// invoked, so a provenance block records what ran rather than what a table says
// ought to have run.
func DeriveExpected(e Expect, in InputTuple, seeds []Seed, bins Bins) (DerivedSet, error) {
	if !KnownExpectKind(e.Kind) {
		return DerivedSet{}, fmt.Errorf("%w: unknown expectation kind %q; refusing to derive nothing "+
			"and report it as a match. Every gate record must carry an expectation this "+
			"package can DERIVE — a stage engraving a shape no ExpectKind names needs a new "+
			"kind here first, not a record minted without one", ErrBadRecord, e.Kind)
	}
	if err := checkStub(e.PolicyIDStub); err != nil {
		return DerivedSet{}, err
	}
	if len(seeds) == 0 {
		return DerivedSet{}, fmt.Errorf("%w: no seeds supplied, so nothing can be derived", ErrIncompleteInputs)
	}
	if len(in.Origins) != len(seeds) {
		return DerivedSet{}, fmt.Errorf("%w: %d origin(s) for %d seed(s) — the tuple cannot say which "+
			"key sits at which path", ErrIncompleteInputs, len(in.Origins), len(seeds))
	}

	var d DerivedSet
	for i, s := range seeds {
		origin := in.Origins[i]
		acct, tmpl, network, err := templateForOrigin(origin)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("seed %q: %w", s.Label, err)
		}
		fp, xpub, gotPath, err := msDerive(bins.MS, s.Words, tmpl, acct, network)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("seed %q: %w", s.Label, err)
		}
		d.note("ms", fmt.Sprintf("ms derive --phrase <seed words: see the inputs file> "+
			"--template %s --account %d --network %s --json", tmpl, acct, network))
		// The recorded origin is a CLAIM. The oracle just derived the path its
		// own template implies; if they differ, the record describes a
		// different key than the one engraved.
		if !samePath(gotPath, origin) {
			return DerivedSet{}, fmt.Errorf("seed %q: the tuple records origin %q but %s derives %q "+
				"for that template — the record and the key material disagree",
				s.Label, origin, tmpl, gotPath)
		}
		chunks, err := mkEncode(bins.MK, xpub, gotPath, fp, e.PolicyIDStub)
		if err != nil {
			return DerivedSet{}, fmt.Errorf("seed %q: %w", s.Label, err)
		}
		d.note("mk", fmt.Sprintf("mk encode --xpub <account xpub from ms derive> "+
			"--origin-path %s --origin-fingerprint <master fp from ms derive> "+
			"--policy-id-stub %s --group-size 0", gotPath, e.PolicyIDStub))
		for _, c := range chunks {
			d.Artifacts = append(d.Artifacts, Artifact{
				Kind: "mk1", Label: s.Label, String: c, Origin: gotPath, Fingerprint: fp,
			})
		}
	}
	return d, nil
}

// CompareCensus checks a census against derived artifacts, byte for byte and in
// order.
//
// Order is part of the answer, not incidental: a bundle is cut card by card and
// plate by plate, and a set that arrives in a different order is a different
// restore than the one the inputs describe.
func CompareCensus(want []Artifact, got []string) error {
	var problems []string
	if len(want) != len(got) {
		problems = append(problems, fmt.Sprintf(
			"the inputs require %d engraved string(s); the census holds %d",
			len(want), len(got)))
	}
	n := len(want)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if want[i].String != got[i] {
			// In full, both of them, always. A digest or a string rendered
			// truncated makes two different values read as one.
			problems = append(problems, fmt.Sprintf(
				"plate %d (%s, %s) differs:\n  expected %s\n  engraved %s",
				i, want[i].Kind, want[i].Label, want[i].String, got[i]))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w:\n%s", ErrRecordMismatch, strings.Join(problems, "\n"))
	}
	if n == 0 {
		return fmt.Errorf("%w: nothing was compared, so this check passed by checking nothing",
			ErrRecordMismatch)
	}
	return nil
}

// pathRe matches an account path down to the BIP-48 script_type level, in
// either notation: m/48'/0'/0'/2' or m/48h/0h/0h/2h.
var pathRe = regexp.MustCompile(`^m/(\d+)['h]/(\d+)['h]/(\d+)['h](?:/(\d+)['h])?$`)

// templateForOrigin maps a recorded origin to the ms template that produces it.
//
// It refuses anything it cannot name, rather than guessing a template and
// deriving a key at a path nobody asked for.
func templateForOrigin(origin string) (account int, template string, network string, err error) {
	m := pathRe.FindStringSubmatch(strings.TrimSpace(origin))
	if m == nil {
		return 0, "", "", fmt.Errorf("origin %q is not an account path this gate can name", origin)
	}
	purpose, _ := strconv.Atoi(m[1])
	coin, _ := strconv.Atoi(m[2])
	account, _ = strconv.Atoi(m[3])
	switch coin {
	case 0:
		network = "mainnet"
	case 1:
		network = "testnet"
	default:
		return 0, "", "", fmt.Errorf("origin %q has coin_type %d, which this gate has no oracle for", origin, coin)
	}
	if purpose != 48 {
		// Single-sig purposes are derivable too, but nothing in this plan
		// engraves one as a cosigner card. Refuse rather than silently widen.
		return 0, "", "", fmt.Errorf("origin %q has purpose %d'; this gate derives BIP-48 cosigner cards only", origin, purpose)
	}
	if m[4] == "" {
		return 0, "", "", fmt.Errorf("origin %q is BIP-48 but names no script_type level", origin)
	}
	switch m[4] {
	case "2":
		template = "bip48-p2wsh"
	case "1":
		template = "bip48-p2sh-p2wsh"
	default:
		return 0, "", "", fmt.Errorf("origin %q has script_type %s', which BIP-48 does not register", origin, m[4])
	}
	return account, template, network, nil
}

// samePath compares two derivation paths across the ' and h notations.
func samePath(a, b string) bool {
	norm := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "h", "'")
	}
	return norm(a) != "" && norm(a) == norm(b)
}

func checkStub(s string) error {
	if len(s) != 8 {
		return fmt.Errorf("%w: policy_id_stub %q must be 8 hex chars (4 bytes)", ErrIncompleteInputs, s)
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return fmt.Errorf("%w: policy_id_stub %q must be lowercase hex", ErrIncompleteInputs, s)
		}
	}
	return nil
}

// msDeriveJSON is the shape `ms derive --json` emits.
type msDeriveJSON struct {
	MasterFingerprint string `json:"master_fingerprint"`
	AccountPath       string `json:"account_path"`
	AccountXpub       string `json:"account_xpub"`
}

// msDerive runs the ms oracle for one seed.
//
// The words go on argv, which ms warns about on stderr. These are BIP-39's
// published vectors and the warning is correct for real material; a gate that
// derived a real seed would pipe them instead.
func msDerive(bin, words, template string, account int, network string) (fp, xpub, path string, err error) {
	if bin == "" {
		return "", "", "", fmt.Errorf("no ms binary resolved")
	}
	out, err := exec.Command(bin, "derive",
		"--phrase", words,
		"--template", template,
		"--account", strconv.Itoa(account),
		"--network", network,
		"--json",
	).Output()
	if err != nil {
		return "", "", "", fmt.Errorf("ms derive --template %s: %w", template, runErr(err))
	}
	var d msDeriveJSON
	if err := json.Unmarshal(out, &d); err != nil {
		return "", "", "", fmt.Errorf("ms derive emitted unparseable JSON: %w", err)
	}
	if d.AccountXpub == "" || d.MasterFingerprint == "" || d.AccountPath == "" {
		return "", "", "", fmt.Errorf("ms derive returned an incomplete account: %+v", d)
	}
	return d.MasterFingerprint, d.AccountXpub, d.AccountPath, nil
}

// mkEncode runs the mk oracle for one card, returning its chunks in order.
//
// --group-size 0 is REQUIRED: mk's default inserts a separator every 5
// characters for display, and a byte comparison against an engraved string must
// see the unbroken form.
func mkEncode(bin, xpub, path, fp, stub string) ([]string, error) {
	if bin == "" {
		return nil, fmt.Errorf("no mk binary resolved")
	}
	out, err := exec.Command(bin, "encode",
		"--xpub", xpub,
		"--origin-path", path,
		"--origin-fingerprint", fp,
		"--policy-id-stub", stub,
		"--group-size", "0",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("mk encode: %w", runErr(err))
	}
	// REFUSE any stdout line that is not an mk1 string, rather than collecting
	// it as one. `mk encode` emits only the strings today — but its sibling
	// `md encode` prints `chunk-set-id: 0x…` on STDOUT ahead of them, so a
	// line-splitter that trusts every line would silently adopt that header as
	// an expected artifact the moment this is extended to md1. An expectation
	// built from a header line compares garbage and reports a mismatch nobody
	// can read; worse, a count that happens to match would pass.
	var chunks []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "mk1") {
			return nil, fmt.Errorf("mk encode emitted a non-mk1 line on stdout, which this "+
				"gate will not treat as an artifact: %q (use --json if the CLI has grown a "+
				"header)", line)
		}
		chunks = append(chunks, line)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("mk encode produced no strings")
	}
	return chunks, nil
}

// runErr surfaces a failed command's stderr, which is where these CLIs put the
// reason. Without it every failure reads "exit status 1".
func runErr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}
