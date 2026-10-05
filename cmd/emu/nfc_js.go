//go:build js

package main

import "syscall/js"

// installNFCAPI exposes the tag source to the page as window.shNFC.
//
//	shNFC.present("<record>")   queue one record, as if held to the reader
//	shNFC.clear()               drop every queued record
//	shNFC.detach()              emulate a machine with NO reader
//	shNFC.attach()              give it a reader again (the default)
//	shNFC.presented()           how many records have crossed the reader
//	shNFC.delivered()           how many records the machine actually READ
//	shNFC.readerAsks()          how many times gui asked the platform for a reader
//	shNFC.forceReader()         Refugium build only: offer gui the reader anyway
//
// presented() is what lets a Build-policy stage gate assert ZERO (F-174). A
// cosigner gather that completed over the emulated reader is green whether or
// not the payload-supplied-cards feature exists, so "the gather finished" is
// not evidence for the feature; "the gather finished AND nothing crossed the
// reader" is.
//
// The justification is SCOPE, not absent hardware (corrected 2026-08-15): the
// SH2 has a soldered ST25R3916, and this phase simply takes its cards from the
// payload and the keyboard instead. Because the reader works, a walk could pass
// by scanning — which is exactly why zero has to be asserted.
//
// It is cumulative for the session and there is NO reset, deliberately — see
// nfc.go. A counter a driver can zero just before asserting is a gate that
// always passes.
//
// A function rather than a bare string so the page cannot leave a tag
// permanently present by assignment, which no physical setup does.
//
// present() QUEUES rather than replaces, which is what lets a walk gather a
// bundle: a screen fetches Platform.NFCReader() once at entry, so every card of
// a gather crosses that one reader. Present them all before entering the flow,
// or present them while it runs -- both work now, and neither did before.
//
// detach/attach exist because "no reader" and "no tag" are different machines
// and gui treats them differently: a nil reader makes flows offer Back-only
// where a scan row would go. A walk must be able to reach that state on
// purpose, and it used to be reachable only by accident -- by having nothing
// queued at the moment a flow started.
func installNFCAPI(n *nfcSource) {
	api := map[string]any{
		"present": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 && args[0].Type() == js.TypeString {
				n.set(args[0].String())
			}
			return nil
		}),
		"clear": js.FuncOf(func(js.Value, []js.Value) any {
			n.set("")
			return nil
		}),
		"detach": js.FuncOf(func(js.Value, []js.Value) any {
			n.detach(true)
			return nil
		}),
		"attach": js.FuncOf(func(js.Value, []js.Value) any {
			n.detach(false)
			return nil
		}),
		"presented": js.FuncOf(func(js.Value, []js.Value) any {
			return n.presented()
		}),
		// delivered() counts reads, not presentations: the Refugium build's
		// NFC-off walk asserts it stays 0 while presented() rises (F7 §4.2).
		"delivered": js.FuncOf(func(js.Value, []js.Value) any {
			return n.delivered()
		}),
		// readerAsks() counts Platform.NFCReader calls; the Refugium walk
		// asserts 0 even with the reader forced on (F7 §4.2, review M-5).
		"readerAsks": js.FuncOf(func(js.Value, []js.Value) any {
			return n.asks()
		}),
		// forceReader() makes the Refugium emulator's NFCReader hand out its
		// reader IF gui asks. gui does not ask under the profile, which
		// readerAsks() proves. No effect on the default build, whose reader
		// is always handed out.
		"forceReader": js.FuncOf(func(js.Value, []js.Value) any {
			n.force()
			return nil
		}),
	}
	js.Global().Set("shNFC", js.ValueOf(api))
}
