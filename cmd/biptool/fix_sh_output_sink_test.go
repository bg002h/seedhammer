package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	osexec "os/exec"
	"strings"
	"syscall"
	"testing"
)

// FIX-sh-go-cli-output-sink (bughunt-1119 bh-sh-I5; ruling T6). biptool takes
// the published failed-write contract (toolkit manual 42-md.md "When stdout
// fails"): a closed pipe only stops the output and the exit code is the
// verb's own; any other stdout write error prints
// "biptool: cannot write the output: <error>" and exits 1.

// TestMain re-executes the test binary as biptool when BIPTOOL_CHILD is set,
// so the exit code and stderr of the real main can be asserted.
func TestMain(m *testing.M) {
	if os.Getenv("BIPTOOL_CHILD") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fixShFullWriter rejects every write the way a full disk does.
type fixShFullWriter struct{}

func (fixShFullWriter) Write([]byte) (int, error) { return 0, syscall.ENOSPC }

// fixShPrefixWriter accepts the first byte of a write, then fails like a disk
// that fills mid-write.
type fixShPrefixWriter struct{}

func (fixShPrefixWriter) Write(p []byte) (int, error) { return min(1, len(p)), syscall.ENOSPC }

type fixShVerb struct {
	name  string
	stdin []byte
	args  string
}

// fixShVerbs is every biptool verb that writes a payload to stdout. The input
// is a codex32 share of all-zero test entropy (no real key).
func fixShVerbs(t *testing.T) []fixShVerb {
	t.Helper()
	share, err := execErr(make([]byte, 16), "seed -id test -seedlen 16")
	if err != nil {
		t.Fatal(err)
	}
	return []fixShVerb{
		{"rand", nil, "rand -n 16"},
		{"seed", make([]byte, 16), "seed -id test -seedlen 16"},
		{"interpolate", share, "interpolate"},
		{"derive seed", share, "derive seed"},
		{"derive bip39", share, "derive -path m bip39 -words 12"},
		{"derive xpub", share, "derive xpub"},
		{"derive xprv", share, "derive xprv"},
		{"derive pubkey", share, "derive pubkey"},
		{"derive privkey", share, "derive privkey"},
	}
}

// fixShRunChild runs biptool (this test binary as main) with the given stdout.
func fixShRunChild(t *testing.T, v fixShVerb, stdout *os.File) (code int, stderr string) {
	t.Helper()
	cmd := osexec.Command(os.Args[0], strings.Split(v.args, " ")...)
	cmd.Env = append(os.Environ(), "BIPTOOL_CHILD=1")
	cmd.Stdin = bytes.NewReader(v.stdin)
	cmd.Stdout = stdout
	var e bytes.Buffer
	cmd.Stderr = &e
	err := cmd.Run()
	var ee *osexec.ExitError
	switch {
	case err == nil:
		return 0, e.String()
	case errors.As(err, &ee):
		if ee.ExitCode() < 0 {
			t.Errorf("biptool %s: %v (killed by a signal, not an exit code)", v.args, err)
		}
		return ee.ExitCode(), e.String()
	default:
		t.Fatalf("biptool %s: %v", v.args, err)
		return 0, ""
	}
}

func TestFixShBiptoolWriteFailureReturnsError(t *testing.T) {
	for _, v := range fixShVerbs(t) {
		t.Run(v.name, func(t *testing.T) {
			if _, err := execErr(v.stdin, v.args); err != nil {
				t.Fatalf("control (working writer): %v", err)
			}
			const want = "cannot write the output: no space left on device"
			for name, w := range map[string]io.Writer{"full": fixShFullWriter{}, "prefix": fixShPrefixWriter{}} {
				err := run(w, bytes.NewReader(v.stdin), strings.Split(v.args, " "))
				if err == nil || err.Error() != want {
					t.Errorf("run with a %s stdout returned %v; want %q", name, err, want)
				}
			}
		})
	}
}

func TestFixShBiptoolFullDiskExit(t *testing.T) {
	for _, v := range fixShVerbs(t) {
		t.Run(v.name, func(t *testing.T) {
			full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
			if err != nil {
				t.Fatalf("open /dev/full: %v", err)
			}
			defer full.Close()
			code, stderr := fixShRunChild(t, v, full)
			const want = "biptool: cannot write the output: write /dev/stdout: no space left on device\n"
			if code != 1 || stderr != want {
				t.Errorf("biptool %s > /dev/full: exit %d stderr %q; want exit 1 stderr %q", v.args, code, stderr, want)
			}
		})
	}
}

func TestFixShBiptoolClosedPipeExit(t *testing.T) {
	for _, v := range fixShVerbs(t) {
		t.Run(v.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			r.Close() // the reader is gone before biptool writes
			defer w.Close()
			code, stderr := fixShRunChild(t, v, w)
			if code != 0 || stderr != "" {
				t.Errorf("biptool %s | (closed reader): exit %d stderr %q; want exit 0 and no stderr", v.args, code, stderr)
			}
		})
	}
}

// Control: a verb's own failure keeps its code and text.
func TestFixShBiptoolOtherErrorUnchanged(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	code, stderr := fixShRunChild(t, fixShVerb{"derive bogus", fixShVerbs(t)[2].stdin, "derive bogus"}, w)
	const want = "biptool: derive: unknown format \"bogus\"\n"
	if code != 2 || stderr != want {
		t.Errorf("biptool derive bogus: exit %d stderr %q; want exit 2 stderr %q", code, stderr, want)
	}
}
