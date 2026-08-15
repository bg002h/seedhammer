package oracle

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests are hermetic: every oracle is a fake binary in t.TempDir() and
// every checkout is a scratch git repo. Nothing here shells out to the real
// md/mk/ms or needs the network, so this stays tier 1 and in the inner loop —
// only real-oracle resolution at gate time is tier 2.

// fakeOracle writes an executable that prints version on --version.
func fakeOracle(t *testing.T, dir, name, version string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := "#!/bin/sh\necho '" + version + "'\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// scratchRepo makes a one-commit git repo and returns its dir and HEAD.
func scratchRepo(t *testing.T) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f")
	run("commit", "-qm", "one")
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return dir, strings.TrimSpace(string(out))
}

// TestOracleHarnessPinsBySourceCommit is the deliverable's central property: a
// binary whose SELF-REPORTED VERSION MATCHES but whose source commit does not
// must be refused.
//
// The test is built so it cannot pass for the wrong reason. It asserts three
// things together: the resolve failed, it failed with ErrCommitMismatch
// specifically, and the version genuinely did match — because a test that only
// checked "it failed" would also pass if the harness refused on version, which
// is the behaviour this deliverable exists to prevent.
func TestOracleHarnessPinsBySourceCommit(t *testing.T) {
	dir, head := scratchRepo(t)
	bin := fakeOracle(t, t.TempDir(), "md", "md 0.42.0")

	const wrongCommit = "0000000000000000000000000000000000000000"
	if wrongCommit == head {
		t.Fatal("scratch repo produced the sentinel commit")
	}

	pin := Pin{Name: "md", Commit: wrongCommit, Version: "md 0.42.0"}
	_, err := Resolve(Request{Pin: pin, Bin: bin, Checkout: dir})
	if err == nil {
		t.Fatal("resolved an oracle whose source commit does not match the pin")
	}
	if !errors.Is(err, ErrCommitMismatch) {
		t.Fatalf("refused for the wrong reason: %v", err)
	}

	// Prove the version really did match, so the refusal above was on commit
	// grounds. Same fake binary, same pin, correct commit -> must resolve.
	got, err := Resolve(Request{Pin: Pin{Name: "md", Commit: head, Version: "md 0.42.0"}, Bin: bin, Checkout: dir})
	if err != nil {
		t.Fatalf("correct commit was refused: %v", err)
	}
	if !got.VersionMatchesPin {
		t.Fatal("the fake oracle's version did not match the pin, so the first case proved nothing")
	}
	if got.Commit != head || got.Method != ByCheckout {
		t.Fatalf("resolved to %s via %s, want %s via %s", got.Commit, got.Method, head, ByCheckout)
	}

	// The converse, and it is the other half of "pins by commit, NOT version":
	// a MISMATCHED version at the CORRECT commit must still resolve. Refusing
	// here would make the harness version-sensitive after all.
	stale := fakeOracle(t, t.TempDir(), "md", "md 0.1.0-stale")
	got, err = Resolve(Request{Pin: Pin{Name: "md", Commit: head, Version: "md 0.42.0"}, Bin: stale, Checkout: dir})
	if err != nil {
		t.Fatalf("a version mismatch at the correct commit was refused: %v", err)
	}
	if got.VersionMatchesPin {
		t.Fatal("expected the version mismatch to be recorded")
	}
	if got.ReportedVersion != "md 0.1.0-stale" {
		t.Fatalf("reported version = %q, want the binary's own claim recorded verbatim", got.ReportedVersion)
	}
}

// TestOracleHarnessRefusesVendoredTestdata — mutation-checked per the plan:
// point it at md/testdata and it must fail.
func TestOracleHarnessRefusesVendoredTestdata(t *testing.T) {
	refused := []string{
		"md/testdata",
		"md/testdata/vectors/wsh_sortedmulti.bytes.hex",
		"../seedhammer/md/testdata/README.md",
		"/abs/path/to/gui/testdata/x.png",
		"md/testdata/", // trailing separator must not evade the check
	}
	for _, p := range refused {
		if err := CheckDataSource(p); !errors.Is(err, ErrVendoredTestdata) {
			t.Errorf("CheckDataSource(%q) = %v, want ErrVendoredTestdata", p, err)
		}
	}

	// And it must accept a real comparison source, or it is not a check, it is
	// a refusal of everything.
	allowed := []string{
		"/scratch/code/shibboleth/descriptor-mnemonic/crates/md-codec",
		"/tmp/gate-run-1/md1.txt",
		"md/encode_multisig.go",
	}
	for _, p := range allowed {
		if err := CheckDataSource(p); err != nil {
			t.Errorf("CheckDataSource(%q) = %v, want nil", p, err)
		}
	}
}

// A dirty checkout must be refused: HEAD does not describe its contents, so
// recording it as the source commit would be recording something untrue.
// Measured 2026-08-14, this is not hypothetical — mnemonic-toolkit had 38
// modified files.
func TestResolveRefusesADirtyCheckout(t *testing.T) {
	dir, head := scratchRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := fakeOracle(t, t.TempDir(), "md", "md 0.42.0")
	_, err := Resolve(Request{Pin: Pin{Name: "md", Commit: head}, Bin: bin, Checkout: dir})
	if !errors.Is(err, ErrDirtyCheckout) {
		t.Fatalf("dirty checkout resolved or failed wrongly: %v", err)
	}
}

func TestResolveByBinaryHash(t *testing.T) {
	bd := t.TempDir()
	bin := fakeOracle(t, bd, "mk", "mk 0.4.2")
	sum, err := fileSHA256(bin)
	if err != nil {
		t.Fatal(err)
	}
	const commit = "5a0a4f41017d71d47f70684c145702d4ca0c3aa9"

	got, err := Resolve(Request{Pin: Pin{Name: "mk", Commit: commit, SHA256: sum}, Bin: bin})
	if err != nil {
		t.Fatalf("matching hash refused: %v", err)
	}
	if got.Method != ByBinaryHash || got.Commit != commit {
		t.Fatalf("got %s via %s", got.Commit, got.Method)
	}

	// A DIFFERENT binary claiming the SAME version must be refused. The
	// substitute has to differ in bytes while being indistinguishable by
	// --version — that is the whole threat. An earlier version of this test
	// built the substitute with the same helper and got a byte-identical file,
	// so it asserted nothing; the extra line is load-bearing.
	other := filepath.Join(t.TempDir(), "mk")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n# substituted\necho 'mk 0.4.2'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if osum, err := fileSHA256(other); err != nil {
		t.Fatal(err)
	} else if osum == sum {
		t.Fatal("the substitute is byte-identical, so this case proves nothing")
	}
	if got := reportedVersion(other); got != "mk 0.4.2" {
		t.Fatalf("substitute reports %q; it must be indistinguishable by --version", got)
	}
	if _, err := Resolve(Request{Pin: Pin{Name: "mk", Commit: commit, SHA256: sum}, Bin: other}); !errors.Is(err, ErrBinaryHashMismatch) {
		t.Fatalf("substituted binary was not refused: %v", err)
	}
}

func TestResolveRefusesAPinThatIdentifiesNothing(t *testing.T) {
	bin := fakeOracle(t, t.TempDir(), "ms", "ms 0.7.0")
	if _, err := Resolve(Request{Pin: Pin{Name: "ms"}, Bin: bin}); !errors.Is(err, ErrIncompletePin) {
		t.Fatalf("empty pin accepted: %v", err)
	}
}

// The gate record must carry COMMITS. A record naming only versions is exactly
// the spoofable artifact this deliverable replaces.
func TestGateRecordCarriesCommitsAndTheInputTuple(t *testing.T) {
	dir, head := scratchRepo(t)
	bin := fakeOracle(t, t.TempDir(), "md", "md 0.42.0")
	res, err := Resolve(Request{Pin: Pin{Name: "md", Commit: head, Version: "md 0.42.0"}, Bin: bin, Checkout: dir})
	if err != nil {
		t.Fatal(err)
	}

	rec := GateRecord{
		Oracles: []Resolved{res},
		Inputs: InputTuple{
			Template: "wsh", N: 2, K: 2,
			SlotOrder: []int{0, 1}, FPChoice: "include",
			Origins: []string{"m/48'/0'/0'/2'", "m/48'/0'/0'/2'"},
			Seeds:   []SeedRef{NewSeedRef("typed:trace-a", "abandon abandon about")},
		},
	}
	b, err := rec.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{head, "\"method\": \"checkout\"", "slot_order", "fp_choice", "origins"} {
		if !strings.Contains(s, want) {
			t.Errorf("gate record is missing %q:\n%s", want, s)
		}
	}
	// The seed words must NOT appear; the digest must.
	if strings.Contains(s, "abandon") {
		t.Errorf("gate record contains seed words:\n%s", s)
	}
	if !strings.Contains(s, NewSeedRef("typed:trace-a", "abandon abandon about").Digest) {
		t.Error("gate record does not carry the seed digest")
	}
	// Same words, same digest; different words, different digest.
	if NewSeedRef("a", "one two").Digest == NewSeedRef("a", "one three").Digest {
		t.Error("seed digests collide across different seeds")
	}
}
