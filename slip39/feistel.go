package slip39

import (
	"crypto/sha256"
	"encoding/binary"

	"golang.org/x/crypto/pbkdf2"
)

const (
	feistelRounds        = 4
	feistelBaseIterCount = 10000
)

// wipe best-effort zeroes a secret-bearing buffer. NOTE: TinyGo's GC may
// copy/retain — defense-in-depth, not a guarantee (SPEC §4.8).
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func itersPerRound(iterationExp int) int {
	return (feistelBaseIterCount << uint(iterationExp)) / feistelRounds
}

func feistelSalt(identifier int, extendable bool) []byte {
	if extendable {
		return nil
	}
	salt := make([]byte, 0, 8)
	salt = append(salt, []byte("shamir")...)
	var idb [2]byte
	binary.BigEndian.PutUint16(idb[:], uint16(identifier))
	return append(salt, idb[:]...)
}

// feistelDecrypt turns the encrypted master secret (EMS) into the master
// secret via the 4-round Feistel run in reverse (rounds 3,2,1,0). Output is
// R||L. Port of feistel.rs decrypt. The passphrase enters ONLY here.
func feistelDecrypt(ems, passphrase []byte, iterationExp, identifier int, extendable bool) []byte {
	n := len(ems)
	half := n / 2
	l := append([]byte(nil), ems[:half]...)
	r := append([]byte(nil), ems[half:]...)
	salt := feistelSalt(identifier, extendable)
	iters := itersPerRound(iterationExp)
	for i := feistelRounds - 1; i >= 0; i-- {
		pw := append([]byte{byte(i)}, passphrase...)
		f := pbkdf2.Key(pw, append(append([]byte(nil), salt...), r...), iters, half, sha256.New)
		for j := 0; j < half; j++ {
			l[j] ^= f[j]
		}
		wipe(pw)
		wipe(f)
		l, r = r, l // swap
	}
	out := append(append([]byte(nil), r...), l...)
	wipe(l)
	wipe(r)
	return out
}
