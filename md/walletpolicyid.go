package md

// ─── WalletPolicyId (PORT of compute_wallet_policy_id, identity.rs:172-240) ──
//
// WalletPolicyId is the 128-bit canonical-expanded policy hash — the mk1 KEY
// card's policy_id_stub source in a bundle (the stub = WalletPolicyId(md1)[0:4],
// SPEC_mk v0.1 §3.3; NOT computeEncodingID, which is the md1 chunk-set-id, a
// DIFFERENT preimage). The fork shipped only computeEncodingID, so this is a
// net-new byte-exact Rust port.
//
// Preimage (R0-I1), built over the CANONICALIZED descriptor:
//
//	canonical_template_tree_bytes              (writeNode(tree) only, byte-padded)
//	‖ for idx in 0..n:
//	      presence_byte (fp_present | xpub_present<<1, masked 0x03)
//	      ‖ record_bytes (a SEPARATE byte-padded bitstream:
//	            varint(path_bit_len) ‖ path_bits
//	            ‖ varint(use_site_bit_len) ‖ use_site_bits)
//	      ‖ fp[4]   (only if fp_present)
//	      ‖ xpub[65] (only if xpub_present)
//
// SHA-256(preimage)[0:16]. A fully-present record is 1+8+4+65 = 78 bytes.
//
// CRITICAL (R0-I2): the per-@N origin/use-site are resolved by the RAW machinery
// here — NOT the display accessor ExpandWalletPolicy (which converts to a
// bip32.Path with in-band hardening AND applies a canonicalOrigin value-fallback
// that Rust expand_per_at_n does NOT). Rust resolves origin as
// override-TLV > path_decl(shared/divergent) value AS-IS, with no canonical
// substitution at hash time (canonicalize.rs:436-455). Mirrored below.
func WalletPolicyId(d *descriptor) ([16]byte, error) {
	// Canonicalize a clone first (identity.rs:173-177).
	dc, err := canonicalize(d)
	if err != nil {
		return [16]byte{}, err
	}

	width := kiw(dc.pathDecl.n)

	// Leading segment: the placeholder-form tree bytes only (writeNode), NOT
	// encode_payload — no header/path_decl/use_site/TLV (identity.rs:179-182).
	var treeW bitWriter
	if err := writeNode(&treeW, dc.tree, width); err != nil {
		return [16]byte{}, err
	}
	preimage := append([]byte(nil), treeW.intoBytes()...)

	// Per-@N records, idx 0..n-1 ascending (identity.rs:185-228).
	for idx := uint8(0); idx < dc.n; idx++ {
		origin, err := resolveOriginRaw(dc, idx)
		if err != nil {
			return [16]byte{}, err
		}
		us := resolveUseSiteRaw(dc, idx)

		// Scratch-write origin + use-site to capture their unpadded bit lengths.
		var pathScratch bitWriter
		if err := writeOriginPath(&pathScratch, origin); err != nil {
			return [16]byte{}, err
		}
		pathBitLen := pathScratch.bitLen()
		pathBytes := append([]byte(nil), pathScratch.intoBytes()...)

		var usScratch bitWriter
		if err := writeUseSitePath(&usScratch, us); err != nil {
			return [16]byte{}, err
		}
		usBitLen := usScratch.bitLen()
		usBytes := append([]byte(nil), usScratch.intoBytes()...)

		// record_bytes: varint(path_bit_len) ‖ path_bits ‖ varint(us_bit_len)
		// ‖ us_bits, in ONE bitstream, byte-padded by intoBytes (identity.rs:199-211).
		var recordW bitWriter
		if err := writeVarint(&recordW, uint32(pathBitLen)); err != nil {
			return [16]byte{}, err
		}
		if err := reEmitBits(&recordW, pathBytes, pathBitLen); err != nil {
			return [16]byte{}, err
		}
		if err := writeVarint(&recordW, uint32(usBitLen)); err != nil {
			return [16]byte{}, err
		}
		if err := reEmitBits(&recordW, usBytes, usBitLen); err != nil {
			return [16]byte{}, err
		}
		recordBytes := recordW.intoBytes()

		fp, fpPresent := fpForId(dc, idx)
		xpub, xpubPresent := xpubForId(dc, idx)
		presence := (b2u(fpPresent) | (b2u(xpubPresent) << 1)) & 0x03

		preimage = append(preimage, presence)
		preimage = append(preimage, recordBytes...)
		if fpPresent {
			preimage = append(preimage, fp[:]...)
		}
		if xpubPresent {
			preimage = append(preimage, xpub[:]...)
		}
	}

	return sha256First16(preimage), nil
}

// WalletPolicyIDStub is the top-4 bytes of WalletPolicyId — the mk1 KEY card's
// policy_id_stub for the md1 POLICY card it belongs to (SPEC_mk v0.1 §3.3).
func WalletPolicyIDStub(d *descriptor) ([4]byte, error) {
	id, err := WalletPolicyId(d)
	if err != nil {
		return [4]byte{}, err
	}
	var stub [4]byte
	copy(stub[:], id[:4])
	return stub, nil
}

// WalletPolicyIdChunks reassembles a chunked md1 set and returns its
// WalletPolicyId — the []string-input form for callers that hold the wire
// strings (the *descriptor is unexported). Reassemble errors surface unchanged.
func WalletPolicyIdChunks(strs []string) ([16]byte, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return [16]byte{}, err
	}
	return WalletPolicyId(d)
}

// WalletPolicyIDStubChunks is the top-4 bytes of WalletPolicyIdChunks — the
// chunked-md1-input form of the mk1 stub source.
func WalletPolicyIDStubChunks(strs []string) ([4]byte, error) {
	id, err := WalletPolicyIdChunks(strs)
	if err != nil {
		return [4]byte{}, err
	}
	var stub [4]byte
	copy(stub[:], id[:4])
	return stub, nil
}

// resolveOriginRaw mirrors expand_per_at_n's origin resolution for the id
// preimage (canonicalize.rs:436-444): the per-@N OriginPathOverrides entry if
// present, else the path_decl value (Divergent[idx] / Shared) AS-IS — NO
// canonicalOrigin fallback (the deliberate divergence from the display accessor,
// R0-I2). The returned originPath may be empty (depth 0): an elided shared path
// under a wrapper that HAS a canonicalOrigin is a legitimate empty origin, and
// is hashed AS-IS.
//
// A decoded descriptor always carries either a shared or a divergent path_decl
// (readPathDecl), so the final no-decl fallthrough is unreachable on the
// public-API path (which is additionally gated by validateExplicitOriginRequired
// upstream). It returns errMissingExplicitOrigin as defense-in-depth against a
// future direct caller passing an author-built AST with neither path_decl set —
// hashing a silent empty origin there would forge a wrong WalletPolicyId.
func resolveOriginRaw(d *descriptor, idx uint8) (originPath, error) {
	if d.tlv.originPresent {
		for _, o := range d.tlv.originOverrides {
			if o.idx == idx {
				return o.path, nil
			}
		}
	}
	if d.pathDecl.divergent != nil {
		if int(idx) < len(d.pathDecl.divergent) {
			return d.pathDecl.divergent[idx], nil
		}
		return originPath{}, nil
	}
	if d.pathDecl.shared != nil {
		return *d.pathDecl.shared, nil
	}
	return originPath{}, errMissingExplicitOrigin
}

// resolveUseSiteRaw mirrors expand_per_at_n's use-site resolution: the per-@N
// override if present, else the descriptor's shared baseline
// (canonicalize.rs:457-460).
func resolveUseSiteRaw(d *descriptor, idx uint8) useSitePath {
	if d.tlv.useSitePresent {
		for _, o := range d.tlv.useSiteOverrides {
			if o.idx == idx {
				return o.path
			}
		}
	}
	return d.useSite
}

// fpForId returns the fingerprint for @idx and whether it is present.
func fpForId(d *descriptor, idx uint8) ([4]byte, bool) {
	if d.tlv.fpPresent {
		for _, e := range d.tlv.fingerprints {
			if e.idx == idx {
				return e.fp, true
			}
		}
	}
	return [4]byte{}, false
}

// xpubForId returns the 65-byte xpub for @idx and whether it is present.
func xpubForId(d *descriptor, idx uint8) ([65]byte, bool) {
	if d.tlv.pubPresent {
		for _, e := range d.tlv.pubkeys {
			if e.idx == idx {
				return e.xpub, true
			}
		}
	}
	return [65]byte{}, false
}
