package seal

import (
	"encoding/hex"
	"testing"
)

// §6.6 — the fixed public-data hash. Port of
// crates/me-cli/src/seal/pubhash.rs's tests, driven by the vector file rather
// than by retyped constants.

// The literals live in vectors.json (pubhash_sealed / pubhash_unsealed) and are
// asserted here as literals, NOT merely as differing from each other. An
// agreement-only assertion is satisfied by any deterministic function of any
// subset of these bytes, because D and E share their public section — verified
// by execution in the Rust suite: hashing only the first record, or pub[:-1],
// both passed the old agreement test.
func TestPublicDataHashMatchesTheVectors(t *testing.T) {
	seen := 0
	for _, v := range loadVectors(t) {
		if v.PubhashSealed == nil {
			// pub_len == 0: §10.2 step 3 displays nothing, so there is no
			// value to pin.
			if v.PubLen != 0 {
				t.Errorf("vector %s has pub_len %d but no pubhash", v.Name, v.PubLen)
			}
			continue
		}
		seen++
		t.Run(v.Name, func(t *testing.T) {
			gotSealed := hex.EncodeToString(sliceOf(PublicDataHash(v.Public, true)))
			if gotSealed != *v.PubhashSealed {
				t.Errorf("sealed hash = %s, vector declares %s", gotSealed, *v.PubhashSealed)
			}
			gotUnsealed := hex.EncodeToString(sliceOf(PublicDataHash(v.Public, false)))
			if gotUnsealed != *v.PubhashUnsealed {
				t.Errorf("unsealed hash = %s, vector declares %s", gotUnsealed, *v.PubhashUnsealed)
			}
			// THE downgrade detector. An earlier spec draft required these to
			// AGREE, which is exactly the blindness a ciphertext-strip needs:
			// strip the ciphertext and tag, zero the crypto fields, set
			// ct_len = 0, and the payload satisfies every §6.2 rule, prompts
			// for no passphrase, and displays the value the operator recorded.
			if gotSealed == gotUnsealed {
				t.Error("sealed and unsealed hashes must DIFFER — that inequality is what makes a downgrade visible")
			}
		})
	}
	if seen != 3 {
		t.Fatalf("expected 3 vectors with a public section (D, E, G), saw %d", seen)
	}
}

// Every byte of the section must matter. This is what kills subset and
// off-by-one mutants, which the sealed-vs-unsealed inequality cannot: D and E
// share a public section, so that inequality moves under the `sealed` byte
// alone.
func TestEveryByteOfTheSectionAffectsTheHash(t *testing.T) {
	for _, name := range []string{"D", "G"} {
		v := vectorNamed(t, name)
		base := PublicDataHash(v.Public, false)

		// §11.4 requires the SECTION's first and last byte, not a record
		// index. Mutating record[0] and record[n-1] anywhere else never varies
		// the section's true first byte, so a hash over input[1:] would
		// survive.
		for _, m := range []struct {
			label  string
			mutate func([]string)
		}{
			{"first byte of the section", func(r []string) {
				r[0] = flipRune(r[0][:1]) + r[0][1:]
			}},
			{"last byte of the section", func(r []string) {
				last := len(r) - 1
				r[last] = r[last][:len(r[last])-1] + flipRune(r[last][len(r[last])-1:])
			}},
		} {
			recs := append([]string(nil), v.Public...)
			m.mutate(recs)
			if PublicDataHash(recs, false) == base {
				t.Errorf("%s: %s must change the hash", name, m.label)
			}
		}
	}
}

// public_record_count is bound into the digest, so a removed record is visible
// and not merely a changed one.
func TestRemovingARecordChangesTheHash(t *testing.T) {
	v := vectorNamed(t, "D")
	if PublicDataHash(v.Public[:len(v.Public)-1], false) == PublicDataHash(v.Public, false) {
		t.Error("dropping a record must change the hash")
	}
}

func TestFormatsInGroupsOfFour(t *testing.T) {
	v := vectorNamed(t, "E")
	// E is the unsealed shape, so what the device DISPLAYS for it is the
	// unsealed digest.
	got := FormatHash(PublicDataHash(v.Public, false))
	want := "70f3 e35a acf7 47db c40f 8376 91aa 61e0"
	if got != want {
		t.Errorf("FormatHash = %q, want %q", got, want)
	}
	if flat := hex.EncodeToString(sliceOf(PublicDataHash(v.Public, false))); flat != *v.PubhashUnsealed {
		t.Errorf("formatted value does not correspond to %s", *v.PubhashUnsealed)
	}
}

func flipRune(s string) string {
	c := s[0]
	if c == 'q' {
		return "p"
	}
	if c == 'm' {
		return "n"
	}
	return "q"
}

func sliceOf(h [16]byte) []byte { return h[:] }
