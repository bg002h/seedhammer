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
	progBip85
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
	progBip85:        {sysw.ClassMnemonic: true, sysw.ClassCodex32Secret: true, sysw.ClassPassphrase: true},
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
func syswFlags(c sysw.Class, src syswSource, sealed, weak bool) []syswFlag {
	var f []syswFlag
	if c.IsSecret() && src == srcPayload && !sealed {
		f = append(f, flagSecretInPlaintext)
	}
	if c.IsSecret() && src == srcPayload && sealed && weak {
		f = append(f, flagWeakPassphrase)
	}
	if src != srcTyped {
		f = append(f, flagSource)
	}
	if c.IsSecret() && src == srcNFC {
		f = append(f, flagNFCNoIntegrity)
	}
	return f
}
