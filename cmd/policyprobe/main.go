// Command policyprobe is the DEVICE LEG of the policy differential harness: it
// answers, for a batch of wallet policies, the question "what addresses does
// THIS DEVICE derive?" — so a driver can compare that against the Rust primary
// and Bitcoin Core.
//
// # Why it exists
//
// The md1 decoder is fuzzed four ways and the codec has property tests on its
// primitives. The POLICY space is untested: nothing generates random wallet
// policies and pushes them through the whole seam — compose, encode to md1,
// decode back, seat keys, derive an address — and compares three independent
// implementations. This is the device's voice in that comparison.
//
// # Why it derives the way it does
//
// It walks the SAME path the inspect screen walks: md.ExpandWalletPolicyChunks,
// then gui.PolicyAddressAt (which is policyAddressAt, exported without a body of
// its own), then at(index, false) and at(index, true). A library shortcut would
// be easier and would measure the wrong thing — a harness that agrees with Rust
// about a path no operator's device takes has proved nothing about the device.
//
// policyAddressAt is the ROUTER, and wrapping the router rather than one of its
// branches is load-bearing. It tries the flat *bip380.Descriptor route first and
// falls back to complexAddressSource. The first cut of this tool wrapped the
// complex branch alone, and so answered "the device declined this policy shape"
// for every single-key, plain-multisig and sh(...) vector in the corpus — eight
// of them — none of which the device declines for that reason. A harness that
// reports the wrong branch's refusal manufactures device findings.
//
// MAINNET ONLY, and there is no flag to change it. The device's policy address
// path is mainnet-only by design (gui/policy_address.go, D1), so a network flag
// here could only be a lie about what the device does.
//
// # I/O contract
//
// One JSON object per line on stdin:
//
//	{"id":"case-0007","chunks":["md1...","md1..."],"indices":[0,1,2]}
//
// One JSON object per line on stdout, same id, in input order, one per input
// line:
//
//	{"id":"case-0007","ok":true,"keys":3,"receive":["bc1q..."],"change":["bc1q..."]}
//	{"id":"case-0008","ok":false,"stage":"expand","error":"md: chunk set incomplete"}
//
// "stage" is exactly one of expand, source, derive — the driver triages by it,
// so it names the stage that actually failed and the three are never collapsed.
// A policy the device cannot derive from is a RESULT, reported as
// stage "source"; it is one of the things this harness exists to count, not an
// error and not a crash.
//
// Results stream: each line is flushed as it is produced, so the tool works as a
// co-process feeding a driver case by case as well as on a whole file.
//
// # Failing loudly instead of fabricating a result
//
// An input line that is not a well-formed case (bad JSON, empty id) is a DRIVER
// bug, not a measurement. Turning one into a policy result would corrupt the
// very counts the harness produces, and skipping it would silently drop a case
// the driver is waiting on. So it names the line on stderr and exits non-zero,
// leaving the results already emitted intact.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"seedhammer.com/gui"
	"seedhammer.com/md"
)

// maxLine caps one input case. md1 chunks run ~80 bytes each, so this is room
// for tens of thousands of them; it exists so a corrupt stream fails with a
// named line instead of being read until memory runs out.
const maxLine = 16 << 20

// caseIn is one policy to probe. Indices are derived on BOTH chains, in the
// order given.
type caseIn struct {
	ID      string   `json:"id"`
	Chunks  []string `json:"chunks"`
	Indices []uint32 `json:"indices"`
}

// caseOut is this device's answer.
//
// Keys is a *int, not an int, so that "the policy seats zero keys" and "expand
// failed so the count is unknown" stay distinguishable: a keyless template is a
// real and interesting source refusal, and omitempty on a plain int would erase
// exactly that case.
//
// Receive and Change are always non-nil on success, so an empty index list
// serialises as [] rather than null.
type caseOut struct {
	ID       string   `json:"id"`
	OK       bool     `json:"ok"`
	Keys     *int     `json:"keys,omitempty"`
	Receive  []string `json:"receive,omitempty"`
	Change   []string `json:"change,omitempty"`
	Stage    string   `json:"stage,omitempty"`
	Error    string   `json:"error,omitempty"`
	Panicked bool     `json:"panic,omitempty"`
}

const (
	stageExpand = "expand"
	stageSource = "source"
	stageDerive = "derive"
)

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64<<10), maxLine)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	line := 0
	for in.Scan() {
		line++
		raw := strings.TrimSpace(in.Text())
		if raw == "" {
			continue // blank separator lines carry no case and produce no result
		}
		var c caseIn
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			fail(out, line, fmt.Sprintf("malformed case: %v", err))
		}
		if c.ID == "" {
			fail(out, line, "case has no id: the driver could not match a result to it")
		}
		if err := enc.Encode(probe(c)); err != nil {
			fmt.Fprintf(os.Stderr, "policyprobe: writing result for %s: %v\n", c.ID, err)
			os.Exit(1)
		}
		// Flush per result: a driver may be feeding this process case by case
		// and waiting on each answer.
		if err := out.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "policyprobe: flushing result for %s: %v\n", c.ID, err)
			os.Exit(1)
		}
	}
	if err := in.Err(); err != nil {
		out.Flush()
		fmt.Fprintf(os.Stderr, "policyprobe: reading stdin after line %d: %v\n", line, err)
		os.Exit(1)
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "policyprobe: flushing stdout: %v\n", err)
		os.Exit(1)
	}
}

// fail reports a malformed input line and stops, keeping every result already
// written.
func fail(out *bufio.Writer, line int, msg string) {
	out.Flush()
	fmt.Fprintf(os.Stderr, "policyprobe: stdin line %d: %s\n", line, msg)
	os.Exit(1)
}

// probe runs one case down the device's path and reports what it found.
//
// A panic anywhere in the codec or the address layer is CAUGHT and reported as
// a failure of the stage it happened in, with panic:true. Two reasons: a
// hostile policy that crashes the device is the most valuable thing this
// harness can find, and losing it — along with every remaining case in the
// batch — to a process death would be the worst way to learn it. panic:true
// keeps it from ever being counted as an ordinary refusal, and the stack goes
// to stderr so it is not reduced to one line.
func probe(c caseIn) (res caseOut) {
	res = caseOut{ID: c.ID}
	stage := stageExpand
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "policyprobe: %s panicked in %s: %v\n%s\n", c.ID, stage, r, debug.Stack())
			res = caseOut{
				ID:       c.ID,
				OK:       false,
				Stage:    stage,
				Error:    fmt.Sprintf("panic: %v", r),
				Panicked: true,
			}
		}
	}()

	tpl, keys, err := md.ExpandWalletPolicyChunks(c.Chunks)
	if err != nil {
		// A card that is ONE md1 string is not a chunk set, and md.Reassemble
		// requires the chunked header: it reads the first four bits as a wire
		// version, finds something else, and reports "md: wire version
		// mismatch". That sentence is true about a header this card does not
		// have and badly false about the card, which the device reads without
		// complaint -- 13 of the 67 vendored vectors are exactly this shape, and
		// every one of them was being counted as a codec failure.
		//
		// So ask the decoder the device asks for a single card. If it reads,
		// the card is fine and the probe carries on with NO keys, which is the
		// literal truth about a keyless template and which the address router
		// will refuse on its own terms. If it does not read, the original chunk
		// error stands: that is the honest error for a card that is neither.
		tpl2, serr := singleStringTemplate(c.Chunks)
		if serr != nil {
			res.Stage = stageExpand
			// For a card that IS one string, the chunk-set error is never the
			// right one to report: it describes a header this card was never
			// supposed to have. sh_wpkh is the case in the corpus -- the device
			// refuses it as "md: missing explicit origin", which is a documented
			// refusal (md/testdata_test.go:35), and reporting it as a wire
			// version mismatch would send a reader looking for a codec bug.
			if errors.Is(serr, errNotOneString) {
				res.Error = err.Error()
			} else {
				res.Error = serr.Error()
			}
			return res
		}
		tpl, keys = tpl2, nil
	}
	n := len(keys)
	res.Keys = &n

	stage = stageSource
	at, ok := gui.PolicyAddressAt(c.Chunks, tpl, keys)
	if !ok {
		res.Stage = stageSource
		res.Error = sourceRefusalNote(keys)
		return res
	}

	stage = stageDerive
	receive := make([]string, 0, len(c.Indices))
	change := make([]string, 0, len(c.Indices))
	for _, idx := range c.Indices {
		a, err := at(idx, false)
		if err != nil {
			res.Stage = stageDerive
			res.Error = fmt.Sprintf("receive[%d]: %v", idx, err)
			return res
		}
		receive = append(receive, a)
	}
	for _, idx := range c.Indices {
		a, err := at(idx, true)
		if err != nil {
			res.Stage = stageDerive
			res.Error = fmt.Sprintf("change[%d]: %v", idx, err)
			return res
		}
		change = append(change, a)
	}

	res.OK = true
	res.Receive = receive
	res.Change = change
	return res
}

// singleStringTemplate reads a card that is one md1 string rather than a chunk
// set, via the same md.Decode the device uses for one.
//
// It is deliberately NOT a fallback for a malformed chunk set: it insists on
// exactly one string, so a genuine multi-chunk failure can never be re-labelled
// as a single-card success. A chunk set that lost a chunk must stay an expand
// failure, because "the operator is missing a card" is the finding.
func singleStringTemplate(chunks []string) (md.Template, error) {
	if len(chunks) != 1 {
		return md.Template{}, errNotOneString
	}
	return md.Decode(chunks[0])
}

// errNotOneString keeps the "several chunks" case from reaching md.Decode,
// whose error would then describe the first chunk rather than the set.
var errNotOneString = errors.New("policyprobe: not a single-string card")

// sourceRefusalNote describes a refusal for the driver's triage.
//
// gui.PolicyAddressAt reports ok/!ok and no reason, so THE VERDICT IS ITS ALONE
// and this text can never change it. What is written here is strictly an
// OBSERVATION of the expanded keys — facts the driver can re-check — and never
// a claim about which branch inside the device fired, because that claim would
// go stale the first time the refusal set changed and would then be a confident
// wrong answer rather than a missing one.
func sourceRefusalNote(keys []md.ExpandedKey) string {
	if len(keys) == 0 {
		return "the device declined: the policy seats no keys, so there is nothing to derive from"
	}
	var missing []string
	for _, k := range keys {
		if !k.XpubPresent {
			missing = append(missing, fmt.Sprintf("@%d", k.Index))
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("the device declined: no xpub for %s", strings.Join(missing, ", "))
	}
	return "the device declined this policy shape: neither the flat descriptor route nor the complex one could derive from it"
}
