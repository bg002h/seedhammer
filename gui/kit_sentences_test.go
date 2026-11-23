package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type kitVecDetail struct {
	Begins     string `json:"begins"`
	Record     int    `json:"record"`
	Index      int    `json:"index"`
	Count      int    `json:"count"`
	FirstCount int    `json:"first_count"`
	Records    []int  `json:"records"`
	Kind       string `json:"kind"`
	Class      string `json:"class"`
}

// kitExpected builds a device sentence from S1 section 14g's table, written
// out independently of kitSentence: placeholders are filled from the vector's
// own 0-based detail, `<n>` counted from 1.
func kitExpected(reason string, d kitVecDetail, recs []string) string {
	R := fmt.Sprintf("Record %d of this payload", d.Record+1)
	r := fmt.Sprintf("record %d of this payload", d.Record+1)
	rec := strings.TrimSpace(recs[d.Record])
	switch reason {
	case "MR_CASE":
		return R + " is an mr1 string with capital letters; the mr1 strings of a kit are all lowercase, as mr prints them."
	case "MR_CHARACTER":
		return R + " begins mr1 but holds a character no mr1 string uses."
	case "MR_LENGTH":
		return fmt.Sprintf("%s begins mr1 but is %d characters long; an mr1 string is 17 to 96 characters long.", R, len(rec))
	case "MR_CHECKSUM":
		return R + " is an mr1 string whose checksum does not match: at least one character is wrong."
	case "MR_RESERVED_SYMBOL":
		return fmt.Sprintf("%s begins %s, which no mr1 card or chunk begins with.", R, rec[:4])
	case "MR_CHUNK_SHORT":
		return R + " is an mr1 chunk too short to hold its chunk header."
	case "MR_CHUNK_PADDING":
		return R + " is an mr1 chunk that does not end the way mr ends a chunk (0 to 4 zero bits after its last byte)."
	case "SET_COUNT":
		return fmt.Sprintf("The mr1 card that begins %s disagrees with itself: %s says the card has %d chunks, but its first chunk says %d.", d.Begins, r, d.Count, d.FirstCount)
	case "SET_INDEX":
		return fmt.Sprintf("The mr1 card that begins %s has %d chunks, numbered 0 to %d, but %s is numbered %d.", d.Begins, d.Count, d.Count-1, r, d.Index)
	case "SET_DUPLICATE":
		return fmt.Sprintf("The mr1 card that begins %s holds chunk %d twice: records %d and %d of this payload. Each copy of a card holds each chunk once, and a copy must be complete before the next copy starts.", d.Begins, d.Index, d.Records[0]+1, d.Records[1]+1)
	case "SET_MISSING":
		return fmt.Sprintf("The mr1 card that begins %s is incomplete: chunk %d of its %d chunks is missing (chunks are numbered from 0), or a chunk repeated after a complete copy began a second, incomplete copy.", d.Begins, d.Index, d.Count)
	case "SET_SLICES":
		return fmt.Sprintf("The mr1 card that begins %s is not divided into chunks the way mr divides a card, so mr did not write it.", d.Begins)
	case "SET_ID":
		return fmt.Sprintf("The mr1 card that begins %s does not match its own chunk-set id: its chunks come from different cards, or one was altered.", d.Begins)
	case "CARD_SPLIT":
		return fmt.Sprintf("The %s card that begins %s is split: %s sits between its strings but is not part of it. Keep each card's strings together.", d.Kind, d.Begins, r)
	case "KIT_OTHER_RECORD":
		return fmt.Sprintf("%s is not an md1, mk1 or mr1 string (it is: %s), so this payload is not a kit.", R, d.Class)
	case "KIT_UNCONFIRMED_MDMK":
		return R + " is an md1 or mk1 string whose card does not decode: it is incomplete or damaged."
	}
	panic("unknown reason " + reason)
}

// TestKitSentencesDevice: every section 14g reason renders with the device
// placeholders over every refused string, set and kit vector.
func TestKitSentencesDevice(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "sysw", "testdata", "sysw_mr_v1_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		StringVectors []struct {
			Name, Record string
			Reason       *string
		} `json:"string_vectors"`
		SetVectors []struct {
			Name    string
			Records []string
			Reason  *string
			Detail  kitVecDetail
		} `json:"set_vectors"`
		KitVectors []struct {
			Name    string
			Records []string
			Reason  *string
			Detail  kitVecDetail
		} `json:"kit_vectors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	check := func(name string, recs []string, reason string, d kitVecDetail) {
		t.Helper()
		v := kitCards(sessionWith(recs...))
		if !v.offers || v.eligible {
			t.Errorf("%s: kit verdict offers=%v eligible=%v, want a refusal", name, v.offers, v.eligible)
			return
		}
		if v.refusal.Reason != reason {
			t.Errorf("%s: reason %s, want %s", name, v.refusal.Reason, reason)
			return
		}
		got, want := kitSentence(v.refusal, recs), kitExpected(reason, d, recs)
		if got != want {
			t.Errorf("%s:\n got  %q\n want %q", name, got, want)
		}
		if strings.ContainsAny(got, "{}<>") {
			t.Errorf("%s: a placeholder is left in %q", name, got)
		}
		seen[reason] = true
	}
	for _, v := range f.StringVectors {
		if v.Reason != nil {
			check(v.Name, []string{v.Record}, *v.Reason, kitVecDetail{Record: 0})
		}
	}
	for _, v := range f.SetVectors {
		if v.Reason != nil {
			check(v.Name, v.Records, *v.Reason, v.Detail)
		}
	}
	for _, v := range f.KitVectors {
		if v.Reason != nil {
			check(v.Name, v.Records, *v.Reason, v.Detail)
		}
	}
	for _, r := range []string{"MR_CASE", "MR_CHARACTER", "MR_LENGTH", "MR_CHECKSUM", "MR_RESERVED_SYMBOL", "MR_CHUNK_SHORT",
		"MR_CHUNK_PADDING", "SET_COUNT", "SET_INDEX", "SET_DUPLICATE", "SET_MISSING", "SET_SLICES", "SET_ID", "CARD_SPLIT",
		"KIT_OTHER_RECORD", "KIT_UNCONFIRMED_MDMK"} {
		if !seen[r] {
			t.Errorf("no vector exercised %s", r)
		}
	}
}
