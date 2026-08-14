package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file exists because cmd/emu carries a REAL sealed payload and the
// passphrase that opens it, and that combination is heading for main.
//
// The records inside it are published test vectors, so no funds are at risk.
// What would be at risk is the claim the emulator makes about itself: a shipped
// SeedHammer II must never boot carrying a pre-known passphrase. Today that
// holds for two structural reasons -- every secret-bearing file is
// //go:build js, and cmd/controller's dependency graph does not contain
// cmd/emu -- and both of those are one careless edit from being false.
//
// Neither of them fails to COMPILE when broken. A `go:build js` deleted from
// sealed_test_payload.go builds fine everywhere. An import of cmd/emu added to
// a firmware package builds fine too. So the confinement gets tests, or it is
// not a property, it is a habit.

// guarded are the identifiers that must not escape cmd/emu's js-only files.
var guarded = []string{
	"sealedTestPassphrase",
	"sealedTestPayload",
	"sealed_test_payload.bin",
	// The passphrase itself, in case someone pastes the words rather than the
	// constant. First two words are enough to catch a copy and short enough
	// not to trip on prose.
	"mosquito neither reopen",
}

// repoRoot is two levels up from cmd/emu, which is where `go test` runs.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("INCONCLUSIVE: ../.. is not the module root (%v), so this test is "+
			"scanning the wrong tree and would pass vacuously", err)
	}
	return root
}

// TestTestPayloadIsConfinedToJSOnlyFiles was REMOVED 2026-08-14. It matched a hand-maintained names[] list
// and an allowed-files list; embed_confinement_test.go now derives both from
// the tree, so the next payload blob is protected without anyone remembering
// to add it. That test also flagged any file merely QUOTING the names in a
// comment, which is what a literal match cannot distinguish.

// packageClause returns the name in src's package declaration. It scans lines
// rather than matching "\npackage main\n", which silently missed every file
// whose package clause is its FIRST line -- both test files in this directory.
func packageClause(src string) string {
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(line, "package "); ok {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// hasJSBuildTag reports whether src carries a `//go:build js` constraint, which
// must appear before the package clause.
func hasJSBuildTag(src string) bool {
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return false
		}
		if strings.HasPrefix(line, "//go:build ") && strings.Contains(line, "js") &&
			!strings.Contains(line, "!js") {
			return true
		}
	}
	return false
}

// TestNothingImportsTheEmulator pins the other half: even a correctly tagged
// file is only safe while nothing a device builds can reach the package it
// lives in.
//
// WHY NOT `go list -deps ./cmd/controller`. That was tried and it cannot work
// here, in two escalating ways that are worth recording because each one looked
// like an answer:
//
//  1. Bare, it fails with "build constraints exclude all Go files" --
//     cmd/controller is `//go:build tinygo && rp` -- and prints NOTHING. Piped
//     into a grep for cmd/emu that reads as zero matches, i.e. as a pass. That
//     is how this confinement was first reported, wrongly.
//  2. With `-tags tinygo,rp` and GOOS=linux GOARCH=arm it prints 255 packages
//     AND still exits non-zero: TinyGo's `machine` and `device/rp` are not in
//     the Go stdlib, so the graph is PARTIAL. A partial graph cannot prove
//     absence -- cmd/emu could sit behind any unresolved edge.
//
// So the invariant is checked where it is total instead. cmd/emu is
// `package main`, which Go forbids importing at all, and no file in the module
// names its import path. Both are decidable from the source with no toolchain,
// no target and no tags -- nothing here can come back empty and be mistaken
// for a pass.
//
// The deeper check -- build the firmware with TinyGo and scan the binary for
// the blob -- was run once by hand and belongs in a release check, not a unit
// test: it costs minutes and needs a toolchain CI does not carry.
//
// Mutations this pins: move the payload into an importable package; add an
// import of seedhammer.com/cmd/emu anywhere.
func TestNothingImportsTheEmulator(t *testing.T) {
	root := repoRoot(t)
	emuDir := filepath.Join(root, "cmd", "emu")
	self := filepath.Join(emuDir, "confinement_test.go")

	// cmd/emu must stay `package main`: an importable package is one refactor
	// away from being imported.
	ents, err := os.ReadDir(emuDir)
	if err != nil {
		t.Fatalf("reading %s: %v", emuDir, err)
	}
	var goFiles int
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".go" {
			continue
		}
		goFiles++
		b, err := os.ReadFile(filepath.Join(emuDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		if packageClause(string(b)) != "main" {
			t.Errorf("cmd/emu/%s is not `package main` -- cmd/emu is unimportable ONLY "+
				"because it is a main package, and that is what keeps the test payload "+
				"out of every other build", e.Name())
		}
	}
	if goFiles == 0 {
		t.Fatal("INCONCLUSIVE: no .go files found in cmd/emu, so this test checked nothing")
	}

	// And nothing anywhere names it.
	const importPath = `"seedhammer.com/cmd/emu"`
	var checked int
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "third_party", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || path == self {
			return nil
		}
		checked++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), importPath) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s imports %s -- the emulator carries a sealed payload and its "+
				"published passphrase, and nothing a device builds may reach it",
				rel, importPath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if checked < 100 {
		t.Fatalf("INCONCLUSIVE: only %d .go files scanned, which is too few to be this "+
			"module -- the walk is looking in the wrong place", checked)
	}
	t.Logf("%d .go files scanned; cmd/emu is package main (%d files) and nothing imports it",
		checked, goFiles)
}
