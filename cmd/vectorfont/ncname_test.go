package main

import (
	"bytes"
	"encoding/xml"
	"os"
	"regexp"
	"testing"
)

// ncName matches an XML NCName: a name with no colon, starting with a
// letter or underscore and containing only letters, digits, '.', '-' and
// '_' thereafter. https://www.w3.org/TR/xml-names/#NT-NCName
//
// This is deliberately ASCII-only (real NCNames also permit a wide range of
// Unicode letters), which is fine here: every id in constant.svg is, and
// should stay, ASCII.
var ncName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// TestConstantSVGIDsAreNCNames guards constant.svg against ids that are not
// valid XML NCNames, such as a bare "!" or "+". Go's encoding/xml parses
// those happily, so a violation here would otherwise go unnoticed until
// someone tries to round-trip the file through a real SVG editor, which may
// not preserve or may mangle a non-NCName id (see mapChar,
// cmd/vectorfont/main.go). Every id must instead be a spelled-out name like
// the existing "zero", "colon", "leftparen", etc. -- see mapChar for the
// full mapping from name to rune.
func TestConstantSVGIDsAreNCNames(t *testing.T) {
	data, err := os.ReadFile("../../font/constant/constant.svg")
	if err != nil {
		t.Fatal(err)
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	var ids []string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		e, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, a := range e.Attr {
			if a.Name.Local == "id" {
				ids = append(ids, a.Value)
			}
		}
	}
	if len(ids) == 0 {
		t.Fatal("no ids found in constant.svg; parser likely broken")
	}
	var bad []string
	for _, id := range ids {
		if !ncName.MatchString(id) {
			bad = append(bad, id)
		}
	}
	if len(bad) > 0 {
		t.Errorf("constant.svg has %d id(s) that are not valid XML NCNames: %q\n"+
			"give each a spelled-out name in mapChar and rename the id to match "+
			"(see e.g. \"exclam\", \"backslash\", \"asciitilde\")", len(bad), bad)
	}
}
