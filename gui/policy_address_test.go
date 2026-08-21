package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"seedhammer.com/address"
	"seedhammer.com/md"
)

// The keyed conformance vectors, as this test needs them: the addresses Rust
// derived, per chain ("0" receive, "1" change).
type policyAddrVector struct {
	Name     string `json:"name"`
	Template string `json:"template"`
	Chains   map[string]struct {
		Addresses []string `json:"addresses"`
	} `json:"chains"`
}

// route names how a wallet policy reached an address on this device.
type route int

const (
	routeNone    route = iota // no address can be shown
	routeFlat                 // via *bip380.Descriptor (address.Receive/Change)
	routeComplex              // via complexAddressSource (taproot tree / wsh miniscript)
)

func (r route) String() string {
	switch r {
	case routeFlat:
		return "flat"
	case routeComplex:
		return "complex"
	default:
		return "none"
	}
}

// routeFor resolves a vector to the route the DEVICE would take for it —
// deliberately in the same order gatheredDescriptorFlow does, so this test
// cannot pass on a route the operator never reaches.
func routeFor(t *testing.T, chunks []string) (route, func(uint32, bool) (string, error)) {
	t.Helper()
	tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	desc, status := expandedToDescriptor(tpl, keys)
	if status == expandOK {
		return routeFlat, func(i uint32, change bool) (string, error) {
			if change {
				return address.Change(desc, i)
			}
			return address.Receive(desc, i)
		}
	}
	if src, ok := complexAddressSource(chunks, keys); ok {
		return routeComplex, src
	}
	return routeNone, nil
}

// TestEveryKeyedVectorReachesAnAddress is the gate on the Stage 4 wiring.
//
// The capability landed in Stage 3 as two package-level APIs with their own
// tests. That is not the same as a device that can show an address: until
// complexAddressSource existed, every one of these policies hit
// gatheredDescriptorFlow's "Complex policy - display only" branch. This test
// asserts the seam from the DEVICE's side — same routing decision, same order —
// so a passing run means an operator sees these addresses, not that a library
// can compute them.
//
// The addresses come from the primary Rust implementation. A route that derives
// SOMETHING is worthless; every index of every chain must match Rust exactly.
func TestEveryKeyedVectorReachesAnAddress(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "md", "testdata", "vectors", "keyed_*.conformance.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no keyed vectors vendored — this gate is checking NOTHING")
	}

	// Shapes this device still cannot derive. EXPLICIT rather than tolerated: a
	// test that lets an undeliverable shape pass quietly is how "display only"
	// outlives the reason for it. Adding a name here must be a deliberate act.
	stillUnsupported := map[string]string{}

	routes := map[string]route{}
	unexpected := []string{}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".conformance.json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var vec policyAddrVector
			if err := json.Unmarshal(raw, &vec); err != nil {
				t.Fatalf("parse: %v", err)
			}
			chunks := loadVectorChunks(t, name)
			r, at := routeFor(t, chunks)
			routes[name] = r

			if why, listed := stillUnsupported[name]; listed {
				if r != routeNone {
					t.Fatalf("%s is listed as unsupported (%s) but now derives via the %s route — "+
						"delete the entry rather than leaving a stale exemption", name, why, r)
				}
				return
			}
			if r == routeNone {
				unexpected = append(unexpected, name+" ("+vec.Template+")")
				t.Fatalf("%s (%s) reaches NO address route — an operator sees \"display only\"", name, vec.Template)
			}

			// Every chain, every index, byte-equal to Rust.
			checked := 0
			chainNames := make([]string, 0, len(vec.Chains))
			for c := range vec.Chains {
				chainNames = append(chainNames, c)
			}
			sort.Strings(chainNames)
			for _, c := range chainNames {
				var change bool
				switch c {
				case "0":
					change = false
				case "1":
					change = true
				default:
					t.Fatalf("unknown chain %q in the vector", c)
				}
				for i, want := range vec.Chains[c].Addresses {
					got, err := at(uint32(i), change)
					if err != nil {
						t.Fatalf("chain %s index %d via %s: %v", c, i, r, err)
					}
					if got != want {
						t.Fatalf("chain %s index %d via %s:\n got  %s\n want %s (rust)", c, i, r, got, want)
					}
					checked++
				}
			}
			if checked == 0 {
				t.Fatalf("%s carries no addresses — the vector proves nothing", name)
			}
			t.Logf("%s: %d addresses via the %s route", name, checked, r)
		})
	}

	if len(unexpected) > 0 {
		t.Errorf("no address route for:\n  %s", strings.Join(unexpected, "\n  "))
	}
	// Report the split, so a regression that quietly moves a shape from the
	// complex route back to none is visible in the log rather than only in a
	// failure.
	var complexCount int
	names := make([]string, 0, len(routes))
	for n := range routes {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "\n  %-32s %s", n, routes[n])
		if routes[n] == routeComplex {
			complexCount++
		}
	}
	t.Logf("routes:%s", b.String())
	if complexCount == 0 {
		t.Error("not one vector took the complex route — the Stage 4 wiring is inert")
	}
}

// vectorAddress returns one address the primary Rust implementation derived.
func vectorAddress(t *testing.T, name string, chain string, index int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "vectors", name+".conformance.json"))
	if err != nil {
		t.Fatalf("read vector %s: %v", name, err)
	}
	var vec policyAddrVector
	if err := json.Unmarshal(raw, &vec); err != nil {
		t.Fatalf("parse vector %s: %v", name, err)
	}
	c, ok := vec.Chains[chain]
	if !ok || index >= len(c.Addresses) {
		t.Fatalf("vector %s has no chain %s index %d", name, chain, index)
	}
	return c.Addresses[index]
}

// TestOperatorReachesComplexPolicyAddresses drives the actual screens.
//
// THIS IS THE TEST THE STAGE IS FOR. complexAddressSource passing its vectors
// proves a function computes addresses; it says nothing about whether anyone can
// get to them. The recurring failure in this repo is exactly that gap — every
// component green, and the call that joins them missing — so this taps the
// button an operator would tap, through the drawer, and reads the address off
// the rendered screen.
//
// tapNavSlot, not click: a click delivers a button event whether or not the
// button was DRAWN, which would pass even if the affordance never appeared.
func TestOperatorReachesComplexPolicyAddresses(t *testing.T) {
	for _, tc := range []struct {
		vector string
		route  string
	}{
		{"keyed_tr_with_leaf", "taproot script path"},
		{"keyed_wsh_thresh", "wsh miniscript"},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			chunks := loadVectorChunks(t, tc.vector)
			want := vectorAddress(t, tc.vector, "0", 0)

			ctx := NewContext(newPlatform())
			frame, drawer, stop := runUITouch(ctx, func() {
				gatheredDescriptorFlow(ctx, &descriptorTheme, chunks)
			})
			defer stop()

			content, ok := frame()
			if !ok {
				t.Fatal("no frame")
			}
			if uiContains(content, "display only") {
				t.Fatalf("%s (%s) still refuses with \"display only\": %q", tc.vector, tc.route, content)
			}
			if !hasNavSlot(ctx, drawer(), Button2) {
				t.Fatalf("%s: no addresses affordance is drawn — the capability is unreachable", tc.vector)
			}
			tapNavSlot(t, ctx, drawer(), Button2)
			if !frameUntil(frame, want, 10) {
				t.Fatalf("%s (%s): the address screen never rendered receive[0] %s", tc.vector, tc.route, want)
			}
		})
	}
}

// TestUnderivableComplexPolicyStillRefuses is the other half, and it is the half
// that keeps the first one honest.
//
// If the Stage 4 routing were wrong in the generous direction — offering the
// addresses button for anything that fails to project onto a bip380.Descriptor —
// the test above would still pass. A policy carrying NO xpubs cannot have an
// address derived from it by anyone, so the refusal must survive, and the button
// must not be drawn.
func TestUnderivableComplexPolicyStillRefuses(t *testing.T) {
	chunks := loadVectorChunks(t, "keyless_tr_with_leaf") // same template, NO xpubs
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, ok := complexAddressSource(chunks, keys); ok {
		t.Fatal("a policy with no xpubs must not claim an address source")
	}

	ctx := NewContext(newPlatform())
	frame, drawer, stop := runUITouch(ctx, func() {
		gatheredDescriptorFlow(ctx, &descriptorTheme, chunks)
	})
	defer stop()
	if !frameUntil(frame, "display only", 8) {
		t.Fatal("a keyless complex policy must still say display only")
	}
	// DISMISS THE ERROR FIRST. Asserting on the nav here would inspect the
	// ERROR screen's single OK button and pass no matter what the policy screen
	// behind it draws — which is exactly what it did, until a mutation that
	// drew the addresses button unconditionally sailed through this test.
	click(&ctx.Router, Button3)
	if !frameUntil(frame, "md1 descriptor", 8) {
		t.Fatal("dismissing the refusal did not reach the read-only policy screen")
	}
	if hasNavSlot(ctx, drawer(), Button2) {
		t.Fatal("an addresses affordance is drawn for a policy no address can be derived from")
	}
}

// TestComplexPolicyScreenNamesWhichWalletID pins plan D2 + D4.
//
// A bare 32-hex id on a screen is ambiguous between the key-STABLE descriptor
// template id and the key-DEPENDENT policy id. An operator comparing the wrong
// one against a coordinator gets a mismatch that looks like a corrupted backup,
// so the label is load-bearing, not decoration.
func TestComplexPolicyScreenNamesWhichWalletID(t *testing.T) {
	chunks := loadVectorChunks(t, "keyed_tr_with_leaf")
	id, err := md.WalletPolicyIdChunks(chunks)
	if err != nil {
		t.Fatalf("WalletPolicyIdChunks: %v", err)
	}
	header := policyIDHeader(chunks)
	if len(header) != 1 {
		t.Fatalf("expected one header line, got %v", header)
	}
	if !strings.Contains(header[0], "Policy id") {
		t.Fatalf("the header does not say WHICH id it is: %q", header[0])
	}
	if !strings.Contains(header[0], hexString(id[:])) {
		t.Fatalf("the header does not carry the policy id %x: %q", id[:], header[0])
	}

	ctx := NewContext(newPlatform())
	frame, _, stop := runUITouch(ctx, func() {
		gatheredDescriptorFlow(ctx, &descriptorTheme, chunks)
	})
	defer stop()
	// The id is 32 hex characters and the screen chunks long lines at 20, so
	// look for a prefix that survives the split rather than the whole string.
	if !frameUntil(frame, "Policy id", 8) {
		t.Fatal("the complex-policy screen does not name the wallet id it shows")
	}
}

func hexString(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0xf])
	}
	return string(out)
}

// TestTheTimelockedTapLeafGapIsCLOSED — the tripwire fired, and this is what it
// became.
//
// `tr(@0, and_v(v:pk(@1), older(144)))` is a timelocked taproot leaf: the shape
// the DESCRIBE path (a three-name vocabulary of pk / multi_a / sortedmulti_a)
// could never name. It used to be pinned as a refusal, with Rust's addresses
// vendored beside it so that a future fix would have something to be right
// against. F-214's emitter closed it, the pinned test failed with "THE GAP IS
// CLOSED", and the address it produced was byte-identical to the vendored one.
//
// Kept as a POSITIVE test rather than deleted: the vector is the only
// timelocked tap leaf in this repo, and it is now the thing that would notice
// if emission regressed to the vocabulary.
func TestTheTimelockedTapLeafGapIsCLOSED(t *testing.T) {
	chunks := loadVectorChunks(t, "gap_tr_leaf_and_v")
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	at, ok := complexAddressSource(chunks, keys)
	if !ok {
		t.Fatal("a timelocked tap leaf no longer derives — F-214 has regressed")
	}
	for _, c := range []struct {
		chain  string
		change bool
	}{{"0", false}, {"1", true}} {
		for i := 0; i < 2; i++ {
			want := vectorAddress(t, "gap_tr_leaf_and_v", c.chain, i)
			got, err := at(uint32(i), c.change)
			if err != nil {
				t.Fatalf("chain %s index %d: %v", c.chain, i, err)
			}
			if got != want {
				t.Fatalf("chain %s index %d:\n got  %s\n want %s (rust)", c.chain, i, got, want)
			}
		}
	}
}

// TestPkhTapLeafGapIsPinnedByShape is the NEW tripwire, and it covers what the
// old one did: complexAddressSource's DERIVE PROBE.
//
// The probe exists so "can this device show addresses for this policy" comes
// from the emitter rather than a hand-kept list. Every conformance vector
// derives, so removing the probe breaks nothing without a shape that cannot —
// which is precisely what the closed gap above stopped being.
//
// `pkh()` in a tap leaf is that shape today. The PRIMARY derives it; this port's
// emitter has no `tagPkH` case, and adding one means hashing the derived key,
// which would pull RIPEMD-160 into a codec that currently does no key work at
// all. Refused rather than approximated.
//
// PINNED BY SHAPE: when the emitter grows `pk_h`, this FAILS saying the gap is
// closed rather than going quiet. Rust's addresses are vendored beside it.
func TestPkhTapLeafGapIsPinnedByShape(t *testing.T) {
	chunks := loadVectorChunks(t, "gap_tr_leaf_pkh")
	tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("the card itself must decode: %v", err)
	}
	// It has to be the PROBE that refuses, not an earlier guard.
	for _, k := range keys {
		if !k.XpubPresent {
			t.Fatal("fixture must carry real xpubs, or it tests the no-keys guard instead")
		}
	}
	if _, ok := expandedKeysToBip380(keys); !ok {
		t.Fatal("fixture must have derivable use-sites, or it tests the use-site guard instead")
	}
	if _, status := expandedToDescriptor(tpl, keys); status != expandUnsupported {
		t.Fatalf("fixture must reach the complex branch, got status %v", status)
	}

	if at, ok := complexAddressSource(chunks, keys); ok {
		want := vectorAddress(t, "gap_tr_leaf_pkh", "0", 0)
		got, err := at(0, false)
		if err == nil && got == want {
			t.Fatalf("THE GAP IS CLOSED: this port now derives %s for a pkh tap leaf, "+
				"matching Rust. Convert this to a positive test, as its predecessor was.", got)
		}
		t.Fatalf("complexAddressSource claims a shape the emitter cannot build "+
			"(got %q, err %v) — Rust derives %s", got, err, want)
	}
}
