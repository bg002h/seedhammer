//go:build !refugium

package gui

// The default (upstream-shaped) build. See profile_refugium.go for what the
// `refugium` tag changes (Refugium plan F7, decision D1).

// refugiumProfile is false in the default build: every Refugium gate is off and
// the firmware behaves as it always has.
const refugiumProfile = false

// proofTriggersEnabled is true in the default build: the typed QA proof
// triggers (prooftriggers_default.go) are live.
const proofTriggersEnabled = true
