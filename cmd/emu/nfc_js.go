//go:build js

package main

import "syscall/js"

// installNFCAPI exposes the tag source to the page as window.shNFC.
//
//	shNFC.present("<record>")   queue one record, as if held to the reader
//	shNFC.clear()               drop every queued record
//	shNFC.detach()              emulate a machine with NO reader
//	shNFC.attach()              give it a reader again (the default)
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
	}
	js.Global().Set("shNFC", js.ValueOf(api))
}
