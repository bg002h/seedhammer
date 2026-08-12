package gui

import (
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
		cs := &ChoiceScreen{
			Title:   "Payload",
			Lead:    "A systemwide payload is present. Load it?",
			Choices: []string{"SKIP", "LOAD"},
		}
		choice, ok := cs.Choose(ctx, th)
		if !ok || choice == 0 {
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
		// A passphrase is up to 24 words (§12.5); the buffer is sized once and
		// inputWordsFlow fills what the operator enters.
		m := make(bip39.Mnemonic, 24)
		// checksumGate FALSE: a passphrase is not a seed and §12.5 puts no
		// checksum on it. terminator TRUE gives the `done` key, because a
		// passphrase has no fixed length to end on.
		//
		// It returns the count of FILLED slots and nothing else, so zero is the
		// only signal for "backed out" -- and it is the right one here: an empty
		// passphrase cannot open anything, and §12.2 already refuses to treat
		// one as absence.
		n := inputWordsFlow(ctx, th, m, 0, "Passphrase",
			wordEntryOpts{checksumGate: false, terminator: true})
		if n == 0 {
			return false
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
		lines := []string{
			"Compare this against what",
			"`me sysw pack` printed:",
			"",
			sysw.FormatHash(d),
		}
		if confirmReviewScreen(ctx, th, "Payload Digest", lines) {
			compared = true
		}
	}

	cliffAbove := pass != "" && sysw.CliffAbove(pass)
	ctx.sysw = &syswSession{}
	ctx.sysw.load(p, identity, h.Sealed(), cliffAbove, compared)

	if !compared {
		// Loaded but not authenticated: `take` refuses, so every program will
		// behave as though there were no payload. Say so, rather than letting
		// the operator discover it as a menu that never appears.
		showError(ctx, th, "Load Payload",
			"Loaded, but not compared — no program will use it. Load again to compare.")
		return true
	}

	// §3.3.3 flags are per-record-class, so they are summarised here over what
	// was actually loaded. None of them refuses anything (§13).
	if lines := syswLoadWarnings(ctx.sysw); len(lines) > 0 {
		confirmReviewScreen(ctx, th, "Payload Warnings", lines)
	}
	return true
}

// syswLoadWarnings renders the payload-level flags once, over the classes that
// are actually present, rather than repeating them at every point of use.
func syswLoadWarnings(s *syswSession) []string {
	var out []string
	seen := map[syswFlag]bool{}
	for _, r := range s.records {
		for _, f := range syswFlags(r.class, srcPayload, s.sealed, s.weak) {
			if seen[f] {
				continue
			}
			seen[f] = true
			switch f {
			case flagSecretInPlaintext:
				out = append(out, "A SECRET is stored unencrypted in flash.")
			case flagWeakPassphrase:
				out = append(out, "The passphrase is below the word-count floor.")
			}
		}
	}
	return out
}
