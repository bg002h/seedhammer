package md

import (
	"encoding/hex"
	"encoding/json"
	"testing"
)

// For a keyed vector, the two stubs a re-minted card carries are the first
// four bytes of the two ids the Rust primary recorded: template id from the
// STRIPPED template, policy id from the keyed chunks.
func TestComposerStubsAreTheTwoIdsFirstFourBytes(t *testing.T) {
	name := "keyed_compose_wsh_sole_sortedmulti"
	var rec struct {
		TemplateID string `json:"wallet_descriptor_template_id"`
		PolicyID   string `json:"wallet_policy_id"`
	}
	if err := json.Unmarshal(vectorRecordFor(t, name), &rec); err != nil {
		t.Fatal(err)
	}
	// Record and card from the same tier: see md/vector_fixtures_test.go.
	keyed := vectorChunksFor(t, name)
	template, err := StripToTemplate(keyed)
	if err != nil {
		t.Fatalf("StripToTemplate: %v", err)
	}
	both, err := ComposerStubs(template, keyed)
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 2 {
		t.Fatalf("stubs = %x, want two", both)
	}
	if got, want := hex.EncodeToString(both[0][:]), rec.TemplateID[:8]; got != want {
		t.Errorf("template stub %s, rust template id starts %s", got, want)
	}
	if got, want := hex.EncodeToString(both[1][:]), rec.PolicyID[:8]; got != want {
		t.Errorf("policy stub %s, rust policy id starts %s", got, want)
	}
	// Template only (no keyed policy yet, §12 item 6): one stub.
	one, err := ComposerStubs(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0] != both[0] {
		t.Fatalf("template-only stubs = %x, want just %x", one, both[0])
	}
}
