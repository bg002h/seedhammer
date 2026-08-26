package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/bip39"
	"seedhammer.com/sysw"
)

// syswLoadFlow reads the SYSTEMWIDE region, opens what it finds, shows the
// operator the number they must compare, and fills the session.
//
// THIS IS THE STEP THE PLAN NEVER HAD (F-144). Every other piece existed:
// Platform.SyswReader() reads 0x10D00000, syswSession stores records, the eight
// programs already ask it for them. Nothing joined the two, so `ctx.sysw` was
// read in seven places and assigned in none, and the feature was inert.
//
// NOT the Sealed Payload. That is a different container in a different region
// (0xE1000000), reached by a different program, and it is FROZEN. Nothing here
// touches seal/, unlock_kdf.go or unlockPayload — including its conditional
// place at the end of the carousel, which its lastNav bound depends on.
//
// Returns true when a payload was loaded into the session.
func syswLoadFlow(ctx *Context, th *Colors, r sysw.Reader, atBoot bool) bool {
	// nil is a supported Platform value, not a stub: a build with no systemwide
	// region says so by returning nil. At boot that must be silent — a machine
	// without a payload has to behave exactly as it did before.
	if r == nil || !r.Probe() {
		if !atBoot {
			showError(ctx, th, "Load Payload",
				"No payload found at 0x10D00000. Write one with `me sysw pack --region`.")
		}
		return false
	}

	if atBoot {
		// LOAD FIRST, so it is the highlighted default: ChoiceScreen opens on
		// index 0, and a payload the operator deliberately wrote to flash is
		// almost always one they mean to use. SKIP stays one tap away, and Back
		// still skips, so nothing is forced -- only the cheap path is the
		// expected one (operator ruling 2026-08-12).
		cs := &ChoiceScreen{
			Title:   "Payload",
			Lead:    "A systemwide payload is present. Load it?",
			Choices: []string{"LOAD", "SKIP"},
		}
		choice, ok := cs.Choose(ctx, th)
		// !ok is Back, which must remain a skip: backing out of a prompt has
		// never meant "yes" anywhere else on this device.
		if !ok || choice == 1 {
			return false
		}
	}

	blob, err := r.Read()
	if err != nil {
		// §5.2: never the words "payload unreadable" for a structural failure —
		// that phrase teaches the operator to read a wrong file as tampering.
		showError(ctx, th, "Load Payload", "Could not read the payload region.")
		return false
	}
	h, err := sysw.ParseHeader(blob)
	if err != nil {
		showError(ctx, th, "Load Payload",
			"There is no systemwide container at 0x10D00000.")
		return false
	}
	if len(blob) < h.TotalLen() {
		showError(ctx, th, "Load Payload",
			"The payload is shorter than its header declares. Nothing was loaded.")
		return false
	}
	region := blob[:h.TotalLen()]
	identity := sysw.Identity(region)

	// [compared] (§12.2) has exactly TWO routes, and this is the only place both
	// are decided. Getting it from one route only is how a payload gets consumed
	// without ever being authenticated.
	var compared bool
	pass := ""

	if h.Sealed() {
		// Route 1: any successful AEAD open, whatever the passphrase. The
		// operator's ruling (§13) is that an open is an open; F2 still tells
		// them it was weakly protected and they proceed.
		// emptyBIP39Mnemonic, NOT make(). inputWordsFlow's empty-slot sentinel is
		// -1, and make() zero-fills — Word(0) is "ABANDON", a real wordlist entry,
		// so a zeroed buffer reads as 24 ALREADY-FILLED slots. Entry then ends
		// immediately and Open receives whatever the zeroes spell instead of what
		// was typed, so NO sealed payload could ever be opened: the AEAD route to
		// [compared] is closed and a sealed pub_len==0 payload is permanently
		// unconsumable. Every other caller in this package uses the helper; this
		// one did not, and that was the whole bug.
		m := emptyBIP39Mnemonic(24)
		var n int
		// §8c: THE COUNT IS CONFIRMED BEFORE THE KDF, and Back is not `done`.
		//
		// Variable-length entry adds a second way to be wrong that looks exactly
		// like the first: a stray `done` at three words of an intended twelve
		// produces a ~31 s wait and an error indistinguishable from having typed
		// the wrong words. The confirmation is the safety, not the key — it makes
		// the truncation visible while it still costs nothing.
		//
		// Back is separate now: it ABORTS the load outright. No KDF, and no error
		// screen, because the operator did not fail to unlock anything — they
		// left. Reporting a wrong passphrase there is the sentence that sends
		// someone hunting a typo in words they never finished typing.
		for {
			var done bool
			// checksumGate FALSE: a passphrase is not a seed and §12.5 puts no
			// checksum on it. terminator TRUE draws the `done` button, because a
			// passphrase has no fixed length to end on.
			//
			// Re-entry resumes at the first FREE slot, not at slot 1 — "with the
			// slots intact" is only true if the next keystroke does not overwrite
			// the first word. Clamped, because a full 24-slot buffer has no free
			// slot to resume at.
			n, done = inputWordsFlow(ctx, th, m, min(n, len(m)-1), "Passphrase",
				wordEntryOpts{checksumGate: false, terminator: true})
			if !done || n == 0 {
				// Back, or `done` on an empty keyboard. An empty passphrase opens
				// nothing and §12.2 refuses to treat one as absence, so both leave.
				return false
			}
			cs := &ChoiceScreen{
				Title:   "Passphrase",
				Lead:    fmt.Sprintf("%d %s - unlock?", n, map[bool]string{true: "word", false: "words"}[n == 1]),
				Choices: []string{"BACK", "UNLOCK"},
			}
			// BACK is choice 0 and therefore the DEFAULT: the confirmation exists
			// because the count may be wrong, so the resting position is the one
			// that costs nothing. Cancelling the screen re-enters entry too — it
			// is the same "no, take me back" the operator just expressed.
			if choice, ok := cs.Choose(ctx, th); ok && choice == 1 {
				break
			}
		}
		words := make([]string, 0, n)
		for i := 0; i < n; i++ {
			words = append(words, strings.ToLower(bip39.LabelFor(m[i])))
		}
		pass = strings.Join(words, " ")
	}

	p, err := sysw.Open(region, pass)
	if err != nil {
		if h.Sealed() {
			showError(ctx, th, "Load Payload",
				"That passphrase did not open this payload.")
		} else {
			showError(ctx, th, "Load Payload", "This payload could not be read.")
		}
		return false
	}
	if h.Sealed() {
		compared = true // the open IS the authentication
	}

	// Route 2: the operator compares the displayed digest. [digest-shown]
	// (§12.4) — shown wherever one EXISTS, that is whenever pub_len > 0, and
	// nowhere else. At pub_len == 0 the digest is a constant every such payload
	// shares, so showing it would invite a comparison that proves nothing.
	if h.PubLen > 0 {
		pub := strings.Split(string(region[sysw.HeaderLen:sysw.HeaderLen+int(h.PubLen)]), "\n")
		d := sysw.PublicDataHash(pub, h.Sealed())
		// G-P3.16 / SPEC §3.2. This named `me sysw pack` -- the WRITE path.
		// Re-running it means re-supplying every record and re-running the
		// whole ceremony, and on the sealed path it mints a fresh passphrase;
		// the operator standing at the machine has the FILE, not the records.
		// `me sysw show <file>` reads what they have and prints this number.
		lines := []string{
			"Compare this against",
			"`me sysw show <file>`",
			"on the host:",
			"",
			sysw.FormatHash(d),
		}
		if confirmReviewScreen(ctx, th, "Payload Digest", lines) {
			compared = true
		}
	}

	cliffAbove := pass != "" && sysw.CliffAbove(pass)
	ctx.sysw = &syswSession{}
	// h.PubLen > 0 is `[digest-shown]` (§12.4) — carried into the session
	// because the UNLOAD confirmation has to say what RELOADING will cost, and a
	// sealed payload with no digest can only be re-compared by entering the
	// passphrase again.
	ctx.sysw.load(p, identity, h.Sealed(), cliffAbove, compared, h.PubLen > 0)

	if !compared {
		// DECLINING THE COMPARISON UNLOADS (operator ruling 2026-08-13). An
		// uncompared session used to survive here, and it was the worst of both
		// states: `has` ignores `compared` so every picker DEFAULTED to the
		// payload, while `take` requires it and refused -- so the operator would
		// press the obvious button and get nothing, in ten places.
		//
		// Dropping the session removes the state rather than teaching ten call
		// sites to avoid it. It also makes the rule the operator asked for true
		// by construction: a payload is a default if and only if it is loaded,
		// and it is only loaded if it was compared.
		ctx.sysw = nil
		showError(ctx, th, "Not Loaded",
			"Digest not compared.\nNothing was loaded.")
		return false
	}

	// §3.3.3 flags are per-record-class, so they are summarised here over what
	// was actually loaded. None of them refuses anything (§13).
	if lines := syswLoadWarnings(ctx.sysw); len(lines) > 0 {
		confirmReviewScreen(ctx, th, "Payload Warnings", lines)
		// §3.3.3's F1 row reads "offers erase". Since §13 D10 there is no erase:
		// the offer is to UNLOAD, which drops the session and leaves the bytes
		// exactly where they are. Offered HERE because F1 is the moment the
		// operator learns a secret is sitting unencrypted in flash, and the
		// answer to "I did not want that" should not be a hunt through the
		// carousel.
		if syswHasFlag(ctx.sysw, flagSecretInPlaintext) {
			cs := &ChoiceScreen{
				Title:   "Payload",
				Lead:    "Keep this payload loaded?",
				Choices: []string{"KEEP", "UNLOAD"},
			}
			if choice, ok := cs.Choose(ctx, th); ok && choice == 1 {
				if syswUnloadFlow(ctx, th) {
					// The session is gone, so nothing was left loaded.
					return false
				}
			}
		}
	}
	return true
}

// syswHasFlag reports whether any loaded record raises `f`, evaluated through
// syswFlags so the rule stays in one place — asking `class.IsSecret()` here
// would be §12.6's secrecy rule written a second time, which is the defect
// shape §12 exists to stop.
func syswHasFlag(s *syswSession, f syswFlag) bool {
	for _, r := range s.records {
		for _, got := range syswFlags(r.class, r.unconfirmed, srcPayload, s.sealed, s.weak) {
			if got == f {
				return true
			}
		}
	}
	return false
}

// syswLoadWarnings renders the payload-level flags once, over the classes that
// are actually present, rather than repeating them at every point of use.
//
// DE-DUPLICATION IS BY (flag, cause), NOT BY FLAG. Since §12.6, F1 has two
// causes -- a genuinely secret class, and a ClassMDMK record the device could
// not confirm -- and they need different sentences. An operator who deliberately
// wrote one card of a chunked set has to be told THAT is what the machine is
// complaining about; the plain sentence would read as data loss on a payload
// that is exactly as intended. Keying `seen` on the flag alone would print
// whichever cause came first and silently drop the other.
func syswLoadWarnings(s *syswSession) []string {
	type cause struct {
		f           syswFlag
		unconfirmed bool
	}
	var out []string
	seen := map[cause]bool{}
	for _, r := range s.records {
		for _, f := range syswFlags(r.class, r.unconfirmed, srcPayload, s.sealed, s.weak) {
			// Only F1 and F2 read through secrecy AND are rendered here, so the
			// cause is only meaningful for them; the key carries it regardless so
			// a future flag cannot inherit the wrong sentence by omission.
			k := cause{f: f, unconfirmed: r.unconfirmed}
			if seen[k] {
				continue
			}
			seen[k] = true
			switch {
			case f == flagSecretInPlaintext && r.unconfirmed:
				out = append(out, "An md1/mk1 the device could not confirm - treated as a "+
					"secret - is stored unencrypted in flash.")
			case f == flagSecretInPlaintext:
				out = append(out, "A SECRET is stored unencrypted in flash.")
			case f == flagWeakPassphrase && r.unconfirmed:
				out = append(out, "An md1/mk1 the device could not confirm - treated as a "+
					"secret - is protected by a passphrase below the word-count floor.")
			case f == flagWeakPassphrase:
				out = append(out, "The passphrase is below the word-count floor.")
			}
		}
	}
	return out
}
