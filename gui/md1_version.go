package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/md"
)

// md1VersionRefusal reports whether err is md's unsupported-wire-version
// refusal (SPEC_liana_unspendable_internal_key §6a), and the version the card
// DECLARES.
func md1VersionRefusal(err error) (uint8, bool) {
	var wv *md.WireVersionError
	if errors.As(err, &wv) {
		return wv.Got, true
	}
	return 0, false
}

// md1VersionMessage is the ONE phrasing every surface shows for that refusal,
// so no two screens say it differently. It states what the card declares and
// what this firmware can do -- never that the card is intact, current or from
// a newer tool: a BCH mis-correction can land on any header value (F-643).
func md1VersionMessage(v uint8) string {
	return fmt.Sprintf("This firmware cannot read md1 version %d.", v)
}

// md1StringVersionRefusal is md1VersionRefusal for a raw md1 string: the
// chunk header for a chunked card, the payload header for a single one
// (md.ParseChunkHeader does not read a single card's version).
func md1StringVersionRefusal(s string) (uint8, bool) {
	if _, err := md.ParseChunkHeader(s); err != nil {
		return md1VersionRefusal(err)
	}
	_, err := md.Decode(s)
	return md1VersionRefusal(err)
}
