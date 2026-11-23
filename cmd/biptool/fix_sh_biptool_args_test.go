package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// FIX-sh-biptool-args (bughunt-1119 bh-sh-I3, bh-sh-I4). `derive` refuses an
// argument left after the format (and after its own -words / -n options), and
// `derive bip39` refuses a seed shorter than the entropy the word count needs.
// Each row runs the real main (TestMain's BIPTOOL_CHILD re-exec), so flag state
// does not leak between rows and the exit code is asserted.

// All inputs are test entropy (no real key): the codex32 share of bytes
// 00..0f, and shares of all-zero bytes made in-process by `seed`.
const (
	fixArgsShare0f = "ms10testsqqqsyqcyq5rqwzqfpg9scrgwpupj0pvk8uetl6l"
	fixArgsXprv    = "xprv9s21ZrQH143K2LBWUUQRFXhucrQqBpKdRRxNVq2zBqsx8HVqFk2uYo8kmbaLLHRdqtQpUm98uKfu3vca1LqdGhUtyoFnCNkfmXRyPXLjbKb"
	fixArgsChild   = "xpub661MyMwAqRbcGYtxqsn5fkBerXLHYPhqTY36WeRoyARM8EQogL2Gw4a18TRg7gkfnQSYDDKzZRNQWMND4Wdo3ojq7Coiu96S4cLhwaALp6H"
)

// fixArgsZeroShare is the codex32 share of n zero bytes.
func fixArgsZeroShare(t *testing.T, n int) []byte {
	t.Helper()
	share, err := execErr(make([]byte, n), "seed -id test -seedlen "+strconv.Itoa(n)+" -threshold 1 -idx S -hrp ms")
	if err != nil {
		t.Fatalf("seed -seedlen %d: %v", n, err)
	}
	return share
}

// fixArgsRun runs biptool in a child process and returns exit code, stdout
// and stderr.
func fixArgsRun(t *testing.T, stdin []byte, args string) (int, string, string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	code, stderr := fixShRunChild(t, fixShVerb{args, stdin, args}, out)
	b, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(b), stderr
}

type fixArgsRefusal struct {
	stdin  []byte
	args   string
	stderr string
}

func fixArgsWantRefusal(t *testing.T, rows []fixArgsRefusal) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.args, func(t *testing.T) {
			code, stdout, stderr := fixArgsRun(t, r.stdin, r.args)
			if code != 2 || stdout != "" || stderr != r.stderr {
				t.Errorf("biptool %s: exit %d stdout %q stderr %q; want exit 2, no stdout, stderr %q",
					r.args, code, stdout, stderr, r.stderr)
			}
		})
	}
}

// R1: an option placed after a flagless format is refused, not dropped.
func TestFixShBiptoolArgsMisplacedPath(t *testing.T) {
	s := []byte(fixArgsShare0f)
	var rows []fixArgsRefusal
	for _, f := range []string{"xpub", "xprv", "pubkey", "privkey"} {
		rows = append(rows, fixArgsRefusal{s, "derive " + f + " -path m/83696968h/32h/0h",
			"biptool: derive: unexpected argument \"-path\" after format \"" + f + "\"\n"})
	}
	fixArgsWantRefusal(t, rows)
}

// R1: any argument left after the format, or after bip39's -words or seed's
// -n, is refused; the first leftover is named.
func TestFixShBiptoolArgsLeftoverArgument(t *testing.T) {
	s := []byte(fixArgsShare0f)
	z32 := fixArgsZeroShare(t, 32)
	fixArgsWantRefusal(t, []fixArgsRefusal{
		{s, "derive xpub extra", "biptool: derive: unexpected argument \"extra\" after format \"xpub\"\n"},
		{z32, "derive -path m bip39 extra -words 12", "biptool: derive: unexpected argument \"extra\" after format \"bip39\"\n"},
		{z32, "derive -path m bip39 -words 12 extra", "biptool: derive: unexpected argument \"extra\" after format \"bip39\"\n"},
		{s, "derive -path m seed extra -n 8", "biptool: derive: unexpected argument \"extra\" after format \"seed\"\n"},
		{s, "derive -path m seed -n 8 extra", "biptool: derive: unexpected argument \"extra\" after format \"seed\"\n"},
	})
}

// R2, R3: bip39 refuses a seed shorter than the word count's entropy.
func TestFixShBiptoolArgsShortSeed(t *testing.T) {
	rows := []fixArgsRefusal{
		{[]byte(fixArgsShare0f), "derive -path m bip39 -words 24",
			"biptool: derive: seed is 16 bytes; -words 24 needs exactly 32 bytes\n"},
		{[]byte(fixArgsXprv), "derive -path m bip39 -words 12",
			"biptool: derive: no seed provided\n"},
	}
	for _, c := range []struct{ seed, words, need int }{{31, 24, 32}, {19, 15, 20}, {23, 18, 24}, {27, 21, 28}} {
		rows = append(rows, fixArgsRefusal{fixArgsZeroShare(t, c.seed), "derive -path m bip39 -words " + strconv.Itoa(c.words),
			"biptool: derive: seed is " + strconv.Itoa(c.seed) + " bytes; -words " +
				strconv.Itoa(c.words) + " needs exactly " + strconv.Itoa(c.need) + " bytes\n"})
	}
	fixArgsWantRefusal(t, rows)
}

// R5 (S1): the other verbs refuse a leftover argument too, before reading
// stdin or writing output.
func TestFixShBiptoolArgsOtherVerbsLeftover(t *testing.T) {
	const ac = "MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM\nMS12NAMECACDEFGHJKLMNPQRSTUVWXYZ023FTR2GDZMPY6PN\n"
	refuse := func(verb string) string {
		return "biptool: " + verb + ": unexpected argument \"extra\"\n"
	}
	fixArgsWantRefusal(t, []fixArgsRefusal{
		{nil, "rand extra -n 64", refuse("rand")},
		{nil, "rand -n 64 extra", refuse("rand")},
		{make([]byte, 16), "seed -id test -seedlen 16 extra -threshold 2 -idx A", refuse("seed")},
		{[]byte(ac), "interpolate extra -idx D", refuse("interpolate")},
		{[]byte(ac), "interpolate -idx D extra", refuse("interpolate")},
	})
}

// R3 (S2): with no BIP-85 path the seed must be exactly the entropy the word
// count takes; a longer seed is refused, not truncated.
func TestFixShBiptoolArgsLongSeed(t *testing.T) {
	fixArgsWantRefusal(t, []fixArgsRefusal{
		{fixArgsZeroShare(t, 32), "derive -path m bip39 -words 12",
			"biptool: derive: seed is 32 bytes; -words 12 needs exactly 16 bytes\n"},
		{fixArgsZeroShare(t, 20), "derive -path m bip39",
			"biptool: derive: seed is 20 bytes; -words 24 needs exactly 32 bytes\n"},
	})
}

// R4 (controls): the documented spellings keep their output.
func TestFixShBiptoolArgsControls(t *testing.T) {
	s := []byte(fixArgsShare0f)
	for _, c := range []struct {
		stdin      []byte
		args, want string
	}{
		{s, "derive -path m/83696968h/32h/0h xpub", fixArgsChild + "\n"},
		{s, "derive -path m bip39 -words 12", "abandon amount liar amount expire adjust cage candy arch gather drum buyer\n"},
		{s, "derive -path m seed -n 8", "\x00\x01\x02\x03\x04\x05\x06\x07"},
		{fixArgsZeroShare(t, 32), "derive -path m bip39 -words 24", strings.Repeat("abandon ", 23) + "art\n"},
	} {
		t.Run(c.args, func(t *testing.T) {
			code, stdout, stderr := fixArgsRun(t, c.stdin, c.args)
			if code != 0 || stdout != c.want || stderr != "" {
				t.Errorf("biptool %s: exit %d stdout %q stderr %q; want exit 0, stdout %q", c.args, code, stdout, stderr, c.want)
			}
		})
	}
}

// R3 (S2) boundary: one byte over the word count's entropy is refused.
func TestFixShBiptoolArgsLongSeedByOne(t *testing.T) {
	fixArgsWantRefusal(t, []fixArgsRefusal{
		{fixArgsZeroShare(t, 17), "derive -path m bip39 -words 12",
			"biptool: derive: seed is 17 bytes; -words 12 needs exactly 16 bytes\n"},
		{fixArgsZeroShare(t, 33), "derive -path m bip39 -words 24",
			"biptool: derive: seed is 33 bytes; -words 24 needs exactly 32 bytes\n"},
	})
}
