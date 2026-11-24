package main

import (
	"bytes"
	"os"
	"testing"
)

// RED: picosign hash, extract and sign -clear on a signed image whose last UF2 block is
// missing (the file ends at a block boundary) refuse with exit 2.
func TestFixShPicosignTruncatedUF2(t *testing.T) {
	file := fixShUF2(t)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data[:len(data)-512], 0o600); err != nil {
		t.Fatal(err)
	}
	for sub, want := range map[string]string{
		"hash":    "picosign: " + file + ": unexpected EOF\n",
		"extract": "picosign: extract: " + file + ": unexpected EOF\n",
		"sign":    "picosign: sign: " + file + ": unexpected EOF\n",
	} {
		args := []string{sub, file}
		if sub == "sign" {
			args = []string{sub, "-clear", file}
		}
		var out bytes.Buffer
		code, stderr := fixShRunChild(t, &out, args...)
		if after, err := os.ReadFile(file); err != nil || !bytes.Equal(after, data[:len(data)-512]) {
			t.Errorf("picosign %s changed the file (err %v)", sub, err)
		}
		if code != 2 || stderr != want || out.Len() != 0 {
			t.Errorf("picosign %s (last block missing): exit %d stderr %q stdout %d bytes; want exit 2, stderr %q, no stdout", sub, code, stderr, out.Len(), want)
		}
	}
}
