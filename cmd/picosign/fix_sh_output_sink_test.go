package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"seedhammer.com/uf2"
)

// FIX-sh-go-cli-output-sink (bughunt-1119 bh-sh-I6; ruling T6). picosign hash
// and extract take the published failed-write contract (toolkit manual
// 42-md.md "When stdout fails"): a closed pipe only stops the output and the
// exit code is the verb's own; any other stdout write error prints
// "picosign: cannot write the output: <error>" and exits 1.

// TestMain re-executes the test binary as picosign when PICOSIGN_CHILD is set.
func TestMain(m *testing.M) {
	if os.Getenv("PICOSIGN_CHILD") == "1" {
		os.Args = append([]string{"picosign"}, os.Args[1:]...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fixShUF2 wraps picobin/testdata/signed.bin.gz in 256-byte UF2 payload blocks.
func fixShUF2(t *testing.T) string {
	t.Helper()
	f, err := os.Open("../../picobin/testdata/signed.bin.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	img, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	n := (len(img) + 255) / 256
	var out []byte
	le := binary.LittleEndian
	for i := 0; i < n; i++ {
		b := make([]byte, 512)
		le.PutUint32(b[0:], 0x0A324655)
		le.PutUint32(b[4:], 0x9E5D5157)
		le.PutUint32(b[8:], 0x2000)
		le.PutUint32(b[12:], 0x10000000+uint32(256*i))
		le.PutUint32(b[16:], 256)
		le.PutUint32(b[20:], uint32(i))
		le.PutUint32(b[24:], uint32(n))
		le.PutUint32(b[28:], uint32(uf2.FamilyRP2350ARMSigned))
		copy(b[32:32+256], img[256*i:min(len(img), 256*(i+1))])
		le.PutUint32(b[508:], 0x0AB16F30)
		out = append(out, b...)
	}
	p := filepath.Join(t.TempDir(), "signed.uf2")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// fixShRunChild runs picosign (this test binary as main) with the given stdout.
func fixShRunChild(t *testing.T, stdout io.Writer, args ...string) (code int, stderr string) {
	t.Helper()
	cmd := osexec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "PICOSIGN_CHILD=1")
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
			t.Errorf("picosign %v: %v (killed by a signal, not an exit code)", args, err)
		}
		return ee.ExitCode(), e.String()
	default:
		t.Fatalf("picosign %v: %v", args, err)
		return 0, ""
	}
}

// Control: the fixture's reference output through a working stdout.
func TestFixShPicosignOutputControl(t *testing.T) {
	file := fixShUF2(t)
	for sub, want := range map[string]string{
		"hash":    "15cf016da39866e8d1c0dff1aaa29fd0429876f9b55a290c1fc6fce819783557",
		"extract": fixShExtractWant,
	} {
		var out bytes.Buffer
		code, stderr := fixShRunChild(t, &out, sub, file)
		got := out.String()
		if sub == "hash" {
			got = hex.EncodeToString(out.Bytes())
		}
		if code != 0 || stderr != "" || got != want {
			t.Errorf("picosign %s: exit %d stderr %q out %q; want exit 0, no stderr, out %q", sub, code, stderr, got, want)
		}
	}
}

func TestFixShPicosignFullDiskExit(t *testing.T) {
	file := fixShUF2(t)
	for _, sub := range []string{"hash", "extract"} {
		t.Run(sub, func(t *testing.T) {
			full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
			if err != nil {
				t.Fatalf("open /dev/full: %v", err)
			}
			defer full.Close()
			code, stderr := fixShRunChild(t, full, sub, file)
			const want = "picosign: cannot write the output: write /dev/stdout: no space left on device\n"
			if code != 1 || stderr != want {
				t.Errorf("picosign %s > /dev/full: exit %d stderr %q; want exit 1 stderr %q", sub, code, stderr, want)
			}
		})
	}
}

func TestFixShPicosignClosedPipeExit(t *testing.T) {
	file := fixShUF2(t)
	for _, sub := range []string{"hash", "extract"} {
		t.Run(sub, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			r.Close() // the reader is gone before picosign writes
			defer w.Close()
			code, stderr := fixShRunChild(t, w, sub, file)
			if code != 0 || stderr != "" {
				t.Errorf("picosign %s | (closed reader): exit %d stderr %q; want exit 0 and no stderr", sub, code, stderr)
			}
		})
	}
}

// Control: a verb's own failure keeps its code and text.
func TestFixShPicosignOtherErrorUnchanged(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.uf2")
	var out bytes.Buffer
	code, stderr := fixShRunChild(t, &out, "hash", missing)
	want := "picosign: open " + missing + ": no such file or directory\n"
	if code != 2 || stderr != want {
		t.Errorf("picosign hash (missing file): exit %d stderr %q; want exit 2 stderr %q", code, stderr, want)
	}
}

const fixShExtractWant = "28510e83c2ab21039c023c4c10967405d6efd7ec15bcd8ed92b92bd029a9da4721404e4bd1424bb2fbc1faf102dd63d1797f54be4c872c53c1a63a6ad4305281\n"
