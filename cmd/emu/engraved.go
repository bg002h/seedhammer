// This file carries NO build tag, for the reason plate.go and screen.go give:
// the JS glue is //go:build js and can never run on this host, and the half
// that decides what counts as engraved can, and is (engraved_test.go).
//
// WHAT IT IS FOR. §4.5 makes an emulator walk the closing gate of every stage,
// and the gate is a BYTE COMPARISON -- the md1/mk1/ms1 strings that came out
// must be the ones the inputs require. Until this file the walk could count
// plates and could not say what was on them: the toolpath recorder decodes
// geometry, gui.PlateAware hands over geometry, and no exported call turns an
// arbitrary md1 back into a plate.
//
// The strings arrive through gui.EngravedAware, an interface that exists in
// this build and NOT in the firmware's. See gui/engraved_hook.go for why the
// string never travels on gui.Plate: doing that would keep seed-derived text
// resident, unwipeable, for the whole ~21-minute cut and silently break
// §10.2.2's early clear of the decrypted record.

package main

import (
	"encoding/json"
	"sync"
)

// engravedRecorder accumulates the strings whose plates were actually engraved.
//
// TWO EVENTS, AND THE SECOND IS THE ONE THAT COUNTS. PlateText announces the
// ids a string was rendered into -- one per engraving variant, since the
// operator has yet to choose -- and PlateEngraved reports that one of them
// finished AND was accepted. A census built from the first alone would report
// plates nobody has: validating a string is not cutting it, and backing out of
// the variant picker is an ordinary thing to do.
type engravedRecorder struct {
	mu sync.Mutex
	// candidates maps a plate id to the string it renders. It grows for the
	// session and is never pruned: a walk validates a handful of strings, and
	// forgetting one would turn a legitimate engrave into an ignored id.
	candidates map[uint64]string
	engraved   []string
	// unknown counts engraved ids nobody announced -- every seed, passphrase
	// and free-text plate, which carry id 0. Kept rather than dropped so a walk
	// that reports NO strings can tell "no constellation plate was cut" from
	// "the hook is not wired up at all".
	unknown int
}

func newEngravedRecorder() *engravedRecorder {
	return &engravedRecorder{candidates: make(map[uint64]string)}
}

// PlateText implements half of gui.EngravedAware.
func (r *engravedRecorder) PlateText(ids []uint64, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		r.candidates[id] = text
	}
}

// PlateEngraved implements the other half.
//
// An id nobody announced is IGNORED rather than guessed at, which is the
// contract gui/engraved_hook.go states and the whole reason plates carry an id.
// Guessing -- attributing a finished plate to whichever string was announced
// last -- would let a walk that validated an md1, backed out, and later cut an
// unrelated seed plate report the md1 as engraved. That inflates the gate,
// which is worse than failing it.
func (r *engravedRecorder) PlateEngraved(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.candidates[id]
	if !ok {
		r.unknown++
		return
	}
	r.engraved = append(r.engraved, s)
}

// Strings returns the engraved strings in the order they were engraved.
//
// ORDER IS PART OF THE ANSWER, not incidental: a bundle is cut card by card and
// plate by plate, and a set that arrives in the wrong order is a different
// restore than the one the walk asked for.
//
// It is CUMULATIVE for the session and has no reset, deliberately. The obvious
// place to hang one -- shToolpath.reset() -- is called by beginPlate on every
// new plate (plate.go), so a census that reset with it would be wiped by the
// second plate of any bundle and report one string where twelve were cut.
// Reload the page to start a fresh walk.
func (r *engravedRecorder) Strings() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.engraved...)
}

// StringsJSON is what the page reads: the engraved strings, plus the counts
// that let a walk tell an empty census apart from a broken one.
func (r *engravedRecorder) StringsJSON() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := struct {
		Strings []string `json:"strings"`
		// Announced is how many plate ids validateMdmk offered. Zero here with
		// a walk that reached an engrave screen means the hook is not wired,
		// not that nothing was cut.
		Announced int `json:"announced"`
		// Unattributed counts finished plates that never came from
		// validateMdmk -- seeds, passphrases, free text. Expected to be
		// non-zero on any walk that cuts one, and NOT an error.
		Unattributed int `json:"unattributed"`
	}{
		Strings:      append([]string{}, r.engraved...),
		Announced:    len(r.candidates),
		Unattributed: r.unknown,
	}
	b, err := json.Marshal(out)
	if err != nil {
		// Marshalling []string and two ints cannot fail; if it somehow does,
		// say so rather than hand the page an empty census it would read as
		// "nothing was engraved".
		return `{"error":"engraved census could not be marshalled: ` + err.Error() + `"}`
	}
	return string(b)
}
