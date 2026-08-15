// Package oracle resolves the primary constellation toolchain to SOURCE
// COMMITS, refuses fork-vendored testdata as a comparison source, and emits the
// gate record that byte-identity gates are anchored to.
//
// # Why this exists (S0 D5)
//
// Every byte-identity gate in the multisig-build-repair plan compares device
// output against the primary Rust toolchain. That comparison is only as
// trustworthy as the claim "this is the primary toolchain", and the obvious way
// to check it does not work:
//
//	$ md --version
//	md 0.13.0
//
// A version string is printed by the binary about itself. A substituted or
// stale binary reports whatever its author chose, so pinning by --version
// launders a device defect through every gate in the plan — the whole gate
// spine defeated by one file on PATH. Measured 2026-08-14: none of md, mk or ms
// exposes a commit at all, by --version or --help. So the commit must come from
// outside the binary, and this package is where that happens.
//
// # The two resolution methods, and why there are two
//
// ByCheckout resolves `git rev-parse HEAD` in a pinned source checkout. It is
// the stronger method — it names the source directly — but it is only valid if
// the checkout is CLEAN. A dirty tree means HEAD does not describe the source
// that produced anything, so Resolve refuses it rather than recording a commit
// that is not the truth. This is not hypothetical: measured 2026-08-14,
// mnemonic-toolkit had 38 modified files and mnemonic-secret 3.
//
// ByBinaryHash resolves a binary by SHA-256 against a recorded hash that the
// pin binds to a commit. Weaker — it trusts whoever recorded the pair — but it
// works when no checkout is available.
//
// In both cases the self-reported version is RECORDED and never TRUSTED: a
// version mismatch is noted in the record, while only a commit or hash mismatch
// refuses. That asymmetry is the entire point of the deliverable.
package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Errors. Callers should test with errors.Is.
var (
	// ErrCommitMismatch is the refusal this package exists for: the source
	// commit is not the pinned one, whatever the binary says about itself.
	ErrCommitMismatch = errors.New("oracle: source commit does not match the pin")
	// ErrDirtyCheckout refuses a checkout whose HEAD does not describe its
	// contents.
	ErrDirtyCheckout = errors.New("oracle: source checkout is dirty, so HEAD does not identify it")
	// ErrBinaryHashMismatch refuses a binary that is not the recorded one.
	ErrBinaryHashMismatch = errors.New("oracle: binary hash does not match the pin")
	// ErrVendoredTestdata refuses fork-vendored testdata as a comparison
	// source. Vendored vectors are a subset of ourselves; agreeing with them
	// proves nothing about the primary.
	ErrVendoredTestdata = errors.New("oracle: refusing fork-vendored testdata as a comparison source")
	// ErrIncompletePin refuses a pin that cannot identify anything.
	ErrIncompletePin = errors.New("oracle: pin names neither a commit nor a binary hash")
	// ErrBinaryOutsideCheckout refuses the unsound pairing described below.
	ErrBinaryOutsideCheckout = errors.New("oracle: binary is not inside the pinned checkout, so the checkout's HEAD does not identify it")
)

// Why ByCheckout requires the binary to live INSIDE the checkout.
//
// This was nearly a hole. `git rev-parse HEAD` reports what a source tree says
// about itself RIGHT NOW; it proves nothing about what produced some other
// file. Pairing `~/.cargo/bin/md` with a path to descriptor-mnemonic and
// recording that commit would be an attestation dressed as a measurement — the
// same shape as trusting `--version`, which is the thing this package exists to
// refuse. Anyone who swapped the binary on PATH would sail through.
//
// So the pairing is made structurally impossible: ByCheckout requires Bin to
// resolve inside Checkout — i.e. the oracle was built there, in this run. To
// use an already-installed binary, pin its SHA-256 instead and accept that the
// commit in that pin is an attestation by whoever recorded the pair, bounded by
// the hash.

// Method is how an oracle's identity was established.
type Method string

const (
	ByCheckout   Method = "checkout"
	ByBinaryHash Method = "binary-sha256"
)

// Pin is the recorded identity of one oracle. Commit is authoritative;
// Version is recorded for the gate record and is never trusted as identity.
type Pin struct {
	Name    string `json:"name"`
	Commit  string `json:"commit"`
	SHA256  string `json:"sha256,omitempty"`
	Version string `json:"version,omitempty"`
}

// Request asks to resolve one oracle. Supply Checkout for ByCheckout, or a
// Pin.SHA256 for ByBinaryHash. Supplying both prefers the checkout, because it
// names the source rather than trusting a recorded pair.
type Request struct {
	Pin      Pin
	Bin      string // path to the oracle binary
	Checkout string // path to a pinned source checkout, if any
}

// Resolved is one oracle's established identity, for the gate record.
type Resolved struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
	Method Method `json:"method"`
	// ReportedVersion is what the binary says about itself. Recorded so a
	// reader can see the discrepancy; never used to accept or refuse.
	ReportedVersion string `json:"reported_version"`
	// VersionMatchesPin is false when the binary's self-report disagrees with
	// the pin. This does NOT cause a refusal — a matching version proves
	// nothing and a mismatching one may just be a stale pin field — but a gate
	// record showing false deserves a human look.
	VersionMatchesPin bool `json:"version_matches_pin"`
}

// Resolve establishes one oracle's identity, or refuses.
func Resolve(r Request) (Resolved, error) {
	if r.Pin.Commit == "" && r.Pin.SHA256 == "" {
		return Resolved{}, fmt.Errorf("%s: %w", r.Pin.Name, ErrIncompletePin)
	}

	out := Resolved{Name: r.Pin.Name}
	// Recorded first, and deliberately before any check, so the gate record
	// shows what the binary claimed even when it is then refused.
	out.ReportedVersion = reportedVersion(r.Bin)
	out.VersionMatchesPin = r.Pin.Version == "" || strings.TrimSpace(out.ReportedVersion) == strings.TrimSpace(r.Pin.Version)

	switch {
	case r.Checkout != "":
		inside, err := binInsideCheckout(r.Bin, r.Checkout)
		if err != nil {
			return Resolved{}, fmt.Errorf("%s: %w", r.Pin.Name, err)
		}
		if !inside {
			return Resolved{}, fmt.Errorf("%s: %s not under %s: %w",
				r.Pin.Name, r.Bin, r.Checkout, ErrBinaryOutsideCheckout)
		}
		clean, head, err := gitState(r.Checkout)
		if err != nil {
			return Resolved{}, fmt.Errorf("%s: %w", r.Pin.Name, err)
		}
		if !clean {
			return Resolved{}, fmt.Errorf("%s (%s): %w", r.Pin.Name, r.Checkout, ErrDirtyCheckout)
		}
		if !commitEqual(head, r.Pin.Commit) {
			return Resolved{}, fmt.Errorf("%s: have %s, pinned %s: %w",
				r.Pin.Name, head, r.Pin.Commit, ErrCommitMismatch)
		}
		out.Commit, out.Method = head, ByCheckout

	default:
		sum, err := fileSHA256(r.Bin)
		if err != nil {
			return Resolved{}, fmt.Errorf("%s: %w", r.Pin.Name, err)
		}
		if !strings.EqualFold(sum, r.Pin.SHA256) {
			return Resolved{}, fmt.Errorf("%s: have %s, pinned %s: %w",
				r.Pin.Name, sum, r.Pin.SHA256, ErrBinaryHashMismatch)
		}
		// The pin binds this hash to a commit; that binding is the identity.
		out.Commit, out.Method = r.Pin.Commit, ByBinaryHash
	}
	return out, nil
}

// CheckDataSource refuses a comparison source that is fork-vendored testdata.
//
// The rule is deliberately blunt — any path with a `testdata` element — because
// a subtle rule here fails open. Vendored vectors are not corrupt; they are an
// older and smaller sample of ourselves, so a gate that accepted them would
// prove agreement with a subset of ourselves rather than with the primary.
func CheckDataSource(path string) error {
	clean := filepath.Clean(path)
	for _, el := range strings.Split(filepath.ToSlash(clean), "/") {
		if el == "testdata" {
			return fmt.Errorf("%s: %w", path, ErrVendoredTestdata)
		}
	}
	return nil
}

// SeedRef identifies a seed without recording it.
//
// DEPARTURE FROM THE PLAN, stated rather than buried: the plan's D5 text asks
// the record to carry "the full input tuple (template, n, k, slot order, fp
// choice, per-slot origins, seeds)". Writing seed WORDS into a gate record
// turns that record into key material — a file that must then be handled like a
// backup, in a repo, in CI logs. A label plus a digest is enough to re-select a
// known test seed and to prove two runs used the same one, which is what
// "reproducible rather than remembered" actually requires.
type SeedRef struct {
	// Label names the seed's source, e.g. "payload:card0" or "typed:trace-a".
	Label string `json:"label"`
	// Digest is sha256 of the seed words, hex, truncated to 16 chars. Enough
	// to compare two runs; not enough to recover anything.
	Digest string `json:"digest"`
}

// NewSeedRef digests seed words into a SeedRef. The words are not retained.
func NewSeedRef(label, words string) SeedRef {
	sum := sha256.Sum256([]byte(strings.TrimSpace(words)))
	return SeedRef{Label: label, Digest: hex.EncodeToString(sum[:])[:16]}
}

// InputTuple is everything needed to say two runs had the same inputs.
type InputTuple struct {
	Template  string    `json:"template"`
	N         int       `json:"n"`
	K         int       `json:"k"`
	SlotOrder []int     `json:"slot_order"`
	FPChoice  string    `json:"fp_choice"`
	Origins   []string  `json:"origins"`
	Seeds     []SeedRef `json:"seeds"`
}

// GateRecord is what a gate emits: which oracles it trusted, established by
// commit, and the exact inputs.
type GateRecord struct {
	Oracles []Resolved `json:"oracles"`
	Inputs  InputTuple `json:"inputs"`
}

// Marshal renders the gate record as indented JSON.
func (g GateRecord) Marshal() ([]byte, error) { return json.MarshalIndent(g, "", "  ") }

func reportedVersion(bin string) string {
	if bin == "" {
		return ""
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// binInsideCheckout reports whether bin resolves to a path inside checkout.
// Both sides are resolved through symlinks first, so a symlink on PATH cannot
// launder a binary into a checkout it did not come from.
func binInsideCheckout(bin, checkout string) (bool, error) {
	if bin == "" {
		return false, fmt.Errorf("no binary path given")
	}
	rb, err := filepath.EvalSymlinks(bin)
	if err != nil {
		return false, err
	}
	rc, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(rc, rb)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// gitState reports whether dir is a clean checkout, and its HEAD.
func gitState(dir string) (clean bool, head string, err error) {
	if _, err := os.Stat(dir); err != nil {
		return false, "", err
	}
	rev, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return false, "", fmt.Errorf("git rev-parse in %s: %w", dir, err)
	}
	st, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return false, "", fmt.Errorf("git status in %s: %w", dir, err)
	}
	return len(strings.TrimSpace(string(st))) == 0, strings.TrimSpace(string(rev)), nil
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// commitEqual compares two commit ids, allowing either to be an abbreviation.
func commitEqual(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	// Refuse absurdly short abbreviations rather than matching half the repo.
	return len(a) >= 7 && strings.HasPrefix(b, a)
}

// PinFile is the on-disk pin set, e.g. oracle/pins.json.
type PinFile struct {
	Pins []FilePin `json:"pins"`
}

// FilePin is a Pin plus the recording context a reader needs to judge how much
// the attestation is worth.
type FilePin struct {
	Pin
	Repo string `json:"repo"`
	// CheckoutCleanWhenRecorded is false when the source tree had uncommitted
	// changes at the moment the pair was recorded, which makes the commit a
	// weaker claim about what produced the binary. Recorded rather than
	// silently dropped.
	CheckoutCleanWhenRecorded bool `json:"checkout_clean_when_recorded"`
}

// LoadPins reads a pin file.
func LoadPins(path string) (PinFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return PinFile{}, err
	}
	var pf PinFile
	if err := json.Unmarshal(b, &pf); err != nil {
		return PinFile{}, fmt.Errorf("%s: %w", path, err)
	}
	if len(pf.Pins) == 0 {
		return PinFile{}, fmt.Errorf("%s: no pins", path)
	}
	return pf, nil
}

// ResolveAll resolves every pin against the binary found for it by locate, and
// returns the results in pin-file order. It resolves ALL pins before returning
// an error, so a gate reports every problem at once rather than one per run.
func ResolveAll(pf PinFile, locate func(name string) (bin, checkout string)) ([]Resolved, error) {
	var out []Resolved
	var errs []error
	for _, fp := range pf.Pins {
		bin, checkout := locate(fp.Name)
		r, err := Resolve(Request{Pin: fp.Pin, Bin: bin, Checkout: checkout})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, r)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}
