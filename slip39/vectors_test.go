package slip39

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

type slip39Vector struct {
	Desc      string
	Mnemonics []string
	MasterHex string
}

// loadVectors reads testdata/slip39_vectors.json — an object keyed by the
// ORIGINAL upstream vector index (string) → 4-tuple [desc, [mnemonics],
// master_hex, xprv]. Returns a map keyed by the integer original index, so the
// test references below address vectors by their canonical upstream number.
func loadVectors(t *testing.T) map[int]slip39Vector {
	t.Helper()
	b, err := os.ReadFile("testdata/slip39_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var raw map[string][]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	out := make(map[int]slip39Vector, len(raw))
	for k, e := range raw {
		idx, err := strconv.Atoi(k)
		if err != nil {
			t.Fatalf("bad vector key %q: %v", k, err)
		}
		var v slip39Vector
		_ = json.Unmarshal(e[0], &v.Desc)
		_ = json.Unmarshal(e[1], &v.Mnemonics)
		_ = json.Unmarshal(e[2], &v.MasterHex)
		out[idx] = v
	}
	return out
}

// vectorShares returns all mnemonics of official vector index idx.
func vectorShares(t *testing.T, idx int) []string {
	t.Helper()
	v, ok := loadVectors(t)[idx]
	if !ok {
		t.Fatalf("vector idx %d not present in testdata", idx)
	}
	return v.Mnemonics
}

// vectorShare returns share `share` of official vector index idx.
func vectorShare(t *testing.T, idx, share int) string {
	t.Helper()
	return vectorShares(t, idx)[share]
}

// vectorSecretHex returns the expected master-secret hex of vector idx ("" if invalid).
func vectorSecretHex(t *testing.T, idx int) string {
	t.Helper()
	v, ok := loadVectors(t)[idx]
	if !ok {
		t.Fatalf("vector idx %d not present in testdata", idx)
	}
	return v.MasterHex
}
