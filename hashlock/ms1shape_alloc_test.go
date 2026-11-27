package hashlock

import (
	"strings"
	"testing"
)

// ms1ShapedOld is the allocating predicate the streaming one replaced, kept
// as the oracle.
func ms1ShapedOld(s string) bool {
	t := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range t {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '-' || r == ',' {
			continue
		}
		b.WriteRune(r)
	}
	t = b.String()
	if len(t) < minMS1Len || !strings.HasPrefix(t, "ms1") {
		return false
	}
	for _, r := range t[3:] {
		if !strings.ContainsRune(bech32Charset, r) {
			return false
		}
	}
	return true
}

// TestMS1ShapedMatchesTheAllocatingOne pins the streaming rewrite to the old
// answers, on both the string and the byte entry points.
func TestMS1ShapedMatchesTheAllocatingOne(t *testing.T) {
	body := "0entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
	cases := []string{
		"", "ms1", "ms1" + body, "MS1" + strings.ToUpper(body),
		"  ms1" + body + "\n", " ms1" + body + " ",
		"ms1" + body[:10] + "-" + body[10:],
		"ms1 " + body[:10] + " , " + body[10:],
		"ms1" + body[:44], "ms1" + body[:45], "ms1" + body[:44] + "b",
		"ms1" + body[:44] + "é", "ms1" + body[:20] + " " + body[20:],
		"ms1" + body[:20] + "\v" + body[20:],
		"mx1" + body, "1ms" + body, "m s 1" + body,
		"ms1-2026-08-23-backup.txt", "correct horse battery staple",
	}
	for _, c := range cases {
		want := ms1ShapedOld(c)
		if got := IsMS1Shaped(c); got != want {
			t.Errorf("IsMS1Shaped(%q) = %v, want %v", c, got, want)
		}
		if got := isMS1ShapedBytes([]byte(c)); got != want {
			t.Errorf("isMS1ShapedBytes(%q) = %v, want %v", c, got, want)
		}
	}
	if !IsMS1Shaped("ms1" + body) {
		t.Fatal("the canonical shape must match")
	}
}

// TestValidatePhraseDoesNotAllocate: the phrase is secret and a Go string
// cannot be wiped, so the rule must not copy it.
func TestValidatePhraseDoesNotAllocate(t *testing.T) {
	phrase := []byte("correct horse battery staple, MS1 not really")
	if n := testing.AllocsPerRun(100, func() { _ = ValidatePhrase(phrase) }); n != 0 {
		t.Fatalf("ValidatePhrase allocated %v times per call", n)
	}
}
