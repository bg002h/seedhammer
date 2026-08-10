package op

import (
	"os"
	"strings"
	"testing"
)

// appendArgs/appendRefs zero the array they outgrow, which is the only thing
// standing between a reallocated Buffer and a complete copy of the rendered
// seed. That property is only as good as the routing: a single site left as a
// raw `b.args = append(...)` reintroduces the whole defect, silently, on
// whichever branch it sits.
//
// No behavioural test can catch that. Verified by mutation: reverting ONE site
// (encodeOp's header append) to a raw append leaves every test in this package
// and in gui/ green, because the unrouted array is unreachable and unzeroed --
// exactly the failure round 0 graded Critical. So the structural property is
// enforced structurally, as a lint over the source.
//
// If a new append site is legitimately needed, route it through the funnel. If
// the funnel itself is ever rewritten, update funnelFuncs.
//
// WHAT THIS CANNOT SEE, because a gate that hides its blind spot is worse than
// no gate: it is a textual scan, so a local-alias form
// (`a := b.args; a = append(a, x); b.args = a`) slips through. It walks every
// .go file in the package rather than op.go alone -- b.args and b.refs are
// package-visible, so an append in buffer_len.go or draw.go would be just as
// damaging and an earlier version of this test would not have looked there.
func TestBufferGrowthIsFunnelled(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var srcs []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		srcs = append(srcs, n)
	}
	if len(srcs) == 0 {
		t.Fatal("INCONCLUSIVE: no package sources found to lint")
	}
	var offenders []string
	for _, src := range srcs {
		offenders = append(offenders, lintFile(t, src)...)
	}
	if len(offenders) > 0 {
		t.Errorf("Buffer backing arrays must only grow through appendArgs/appendRefs, "+
			"which zero the array they outgrow. %d unrouted append(s):\n\t%s",
			len(offenders), strings.Join(offenders, "\n\t"))
	}
}

func lintFile(t *testing.T, src string) []string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	lines := strings.Split(string(b), "\n")

	// The two functions permitted to append to the backing arrays.
	funnelFuncs := []string{
		"func (b *Buffer) appendArgs(",
		"func (b *Buffer) appendRefs(",
	}
	inFunnel := false
	var offenders []string
	for i, ln := range lines {
		for _, f := range funnelFuncs {
			if strings.HasPrefix(ln, f) {
				inFunnel = true
			}
		}
		if inFunnel && ln == "}" {
			inFunnel = false
			continue
		}
		if inFunnel {
			continue
		}
		code := ln
		if k := strings.Index(code, "//"); k >= 0 {
			code = code[:k]
		}
		if strings.Contains(code, ".args = append(") || strings.Contains(code, ".refs = append(") {
			offenders = append(offenders, src+":"+itoa(i+1)+": "+strings.TrimSpace(ln))
		}
	}
	return offenders
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [12]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}
