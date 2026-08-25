package gui

import "seedhammer.com/sysw"

// Admission — SPEC_systemwide_payloads §3.3, transcribed.
//
// THE SHAPE IS TWO RULES, NOT ONE MATRIX. The obvious design is
// (class, container, program), and it is wrong: the container variant does NOT
// gate admission — decision 6 lets the plaintext variant carry any class the
// sealed one may. What the container changes is whether a FLAG is raised.
//
//	admission is (class -> program). The container selects flags, never admission.
//
// Two rules, each testable alone, instead of one matrix with a redundant axis.

type syswProgram int

const (
	progBackupWallet syswProgram = iota
	progPassword
	progText
	progXpub
	progBundle
	progSingleSig
	progMultisig
	progWalletPolicy
	progBip85
	progTransaction
)

// admitted is §3.3.2's table. `true` = admitted; absent = refused with a reason.
var admitted = map[syswProgram]map[sysw.Class]bool{
	progBackupWallet: {sysw.ClassMnemonic: true, sysw.ClassCodex32Secret: true},
	progPassword:     {sysw.ClassPassphrase: true},
	progText:         {sysw.ClassFreeText: true},
	progXpub:         {sysw.ClassMnemonic: true, sysw.ClassCodex32Secret: true, sysw.ClassPassphrase: true},
	progBundle:       {sysw.ClassDescriptor: true, sysw.ClassMDMK: true},
	progSingleSig:    {sysw.ClassMnemonic: true, sysw.ClassCodex32Secret: true, sysw.ClassPassphrase: true, sysw.ClassMDMK: true},
	progMultisig:     {sysw.ClassMnemonic: true, sysw.ClassCodex32Secret: true, sysw.ClassPassphrase: true, sysw.ClassDescriptor: true, sysw.ClassMDMK: true},
	// NO seed class. The Wallet Policy program never derives from a secret: its
	// proof is addresses derived from the policy's OWN public keys plus a named
	// wallet id, so admitting a mnemonic would grant a capability the flow has
	// no use for. Least privilege, and it is enforced here rather than by the
	// flow declining to ask.
	progWalletPolicy: {sysw.ClassDescriptor: true, sysw.ClassMDMK: true},
	progBip85:        {sysw.ClassMnemonic: true, sysw.ClassCodex32Secret: true, sysw.ClassPassphrase: true},
	// Engrave Transaction consumes exactly the two transaction record forms.
	// NO seed class and no passphrase: the program signs nothing and derives
	// nothing, so admitting a secret would grant a capability the flow has no
	// use for — least privilege, same reasoning as progWalletPolicy.
	progTransaction: {sysw.ClassMt: true, sysw.ClassTx: true},
}

// admits reports whether a program may consume a class.
//
// SOURCE IS NOT AN INPUT HERE. An NFC-delivered record is admitted by the same
// table, by class, exactly as a payload-delivered one — otherwise the path §5.4
// removed all integrity checking from would escape the one admission function.
// Source is a FLAG input; see syswFlags.
func admits(p syswProgram, c sysw.Class) bool {
	return admitted[p][c]
}

// syswSource is where a record came from. It selects flags, never admission.
type syswSource int

const (
	srcTyped syswSource = iota
	srcNFC
	srcPayload
	// srcDerived is "carried from this session's own derivation" (S6b spec
	// 2.2, R-C/R-J): a value this device itself computed moments ago --
	// never typed, scanned or loaded from a payload. It is neither
	// srcNFC nor srcPayload, so syswFlags' two payload-plaintext checks and
	// its NFC-integrity check never fire for it; it is not srcTyped, so
	// flagSource DOES fire and the acceptance screen names it (R-C.3/R-D).
	// See syswSourceName's switch for the case this value REQUIRES: the
	// `default:` arm resolves to "the keyboard", so a value added without
	// its own case there becomes a printed falsehood with no compile error
	// and no failing test (R-D).
	srcDerived
)

// syswFlag is a screen-level warning. NONE of these refuses anything: the
// operator is told and proceeds (spec §13).
type syswFlag int

const (
	// F1: a secret sits unencrypted in flash.
	flagSecretInPlaintext syswFlag = iota
	// F2: a secret is protected by a passphrase that is not [cliff]-above.
	// [cliff] is a WORD COUNT, so this does not mean the passphrase is strong
	// when unset.
	flagWeakPassphrase
	// F3: the source, at the point of use — for anything not typed.
	flagSource
	// F4: an NFC secret arrived with NO integrity check at all. §5.4 scopes
	// digest verification to flash, so nothing stands behind a tag's contents.
	flagNFCNoIntegrity
)

// syswFlags evaluates §3.3.3 AFTER admission, never as part of it. Each rule is
// independent and more than one can fire.
//
// `unconfirmed` is `[mdmk-decode]` (§12.6), and §3.3.3's 2026-08-12 amendment is
// the whole of its effect here: SECRECY IN F1, F2 AND F4 READS THROUGH IT. A
// ClassMDMK record the device could not reassemble-and-decode may hold anything,
// including the 32 bytes of seed entropy §5.3.2 names, so it counts as secret
// for flags. It does NOT change admission -- §3.3.2's table is class-only, and a
// confirmation input there would be the container axis §3.3 removed, arriving by
// another door.
//
// F3 is untouched: it is "always, for anything not typed" and says nothing about
// secrecy.
func syswFlags(c sysw.Class, unconfirmed bool, src syswSource, sealed, weak bool) []syswFlag {
	// The ONE place the two routes to secrecy are joined. Writing
	// `c.IsSecret() || unconfirmed` at each of the three sites below is how one
	// of them ends up not being updated.
	secret := c.IsSecret() || unconfirmed
	var f []syswFlag
	if secret && src == srcPayload && !sealed {
		f = append(f, flagSecretInPlaintext)
	}
	if secret && src == srcPayload && sealed && weak {
		f = append(f, flagWeakPassphrase)
	}
	if src != srcTyped {
		f = append(f, flagSource)
	}
	if secret && src == srcNFC {
		f = append(f, flagNFCNoIntegrity)
	}
	return f
}
