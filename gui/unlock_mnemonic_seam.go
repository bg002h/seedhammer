package gui

import "seedhammer.com/bip39"

// unlockMnemonicParsedHook fires immediately after unlockEngraveMnemonic's
// `defer clear(m)` is registered, so a test can hold the SAME backing array the
// defer will zero and assert it was zeroed on every early return.
//
// unlockMnemonicHook cannot do this: it has one call site
// (gui/unlock_session.go), on the success path after clear(m), which no
// early return reaches. A test built on it ranges over nil, asserts nothing,
// and passes with the defer deleted.
var unlockMnemonicParsedHook func(bip39.Mnemonic)
