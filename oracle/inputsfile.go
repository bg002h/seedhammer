package oracle

// Reading the operator-authored inputs file.
//
// It is parsed in TWO places for two purposes — cmd/gaterecord builds a record
// from it, and the expectation deriver needs the seed WORDS and the expect
// block — so the shape lives here rather than being declared twice and drifting.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// InputsFile is the on-disk shape of an `-inputs` file.
type InputsFile struct {
	Payload Payload `json:"payload"`
	Inputs  struct {
		Template  string   `json:"template"`
		N         int      `json:"n"`
		K         int      `json:"k"`
		SlotOrder []int    `json:"slot_order"`
		FPChoice  string   `json:"fp_choice"`
		Origins   []string `json:"origins"`
		Seeds     []struct {
			Label  string `json:"label"`
			Words  string `json:"words,omitempty"`
			Digest string `json:"digest,omitempty"`
		} `json:"seeds"`
	} `json:"inputs"`
	// Expect drives the artifact derivation. Optional: a record made before
	// this existed still parses, and the gate says so rather than silently
	// deriving nothing.
	Expect *Expect `json:"expect,omitempty"`
}

// LoadInputsFile reads and structurally checks an inputs file.
//
// Unknown fields are an ERROR: an inputs file with a misspelt key would
// otherwise produce a record missing that input, which is the exact class of
// silence this deliverable exists to remove.
func LoadInputsFile(path string) (InputsFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return InputsFile{}, err
	}
	var inf InputsFile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inf); err != nil {
		return InputsFile{}, fmt.Errorf("%s: %w", path, err)
	}
	return inf, nil
}

// Tuple projects the file onto the InputTuple a record carries, digesting seed
// words so they never reach disk.
func (inf InputsFile) Tuple() (InputTuple, error) {
	t := InputTuple{
		Template:  inf.Inputs.Template,
		N:         inf.Inputs.N,
		K:         inf.Inputs.K,
		SlotOrder: inf.Inputs.SlotOrder,
		FPChoice:  inf.Inputs.FPChoice,
		Origins:   inf.Inputs.Origins,
	}
	for _, s := range inf.Inputs.Seeds {
		switch {
		case s.Words != "" && s.Digest != "":
			return InputTuple{}, fmt.Errorf("seed %q gives both words and a digest; give one", s.Label)
		case s.Words != "":
			t.Seeds = append(t.Seeds, NewSeedRef(s.Label, s.Words))
		case s.Digest != "":
			t.Seeds = append(t.Seeds, SeedRef{Label: s.Label, Digest: s.Digest})
		default:
			return InputTuple{}, fmt.Errorf("seed %q identifies nothing: give words or a digest", s.Label)
		}
	}
	return t, nil
}

// SeedWords returns the seeds that carry words, for the expectation deriver.
//
// A seed recorded as a bare DIGEST cannot be derived from — that is the point
// of allowing a digest — so this refuses rather than deriving a short set and
// comparing it against a full census, which would fail confusingly at the count
// instead of saying why.
func (inf InputsFile) SeedWords() ([]Seed, error) {
	var out []Seed
	for _, s := range inf.Inputs.Seeds {
		if s.Words == "" {
			return nil, fmt.Errorf("%w: seed %q is recorded as a digest only, so the expected "+
				"artifacts cannot be derived from it; supply words, or do not gate this record "+
				"on a derived census", ErrIncompleteInputs, s.Label)
		}
		out = append(out, Seed{Label: s.Label, Words: s.Words})
	}
	return out, nil
}
