//go:build js

package main

import "syscall/js"

// installNFCAPI exposes the tag source to the page as window.shNFC.
//
//	shNFC.present("<record>")   queue one record, as if held to the reader
//	shNFC.clear()               remove it
//
// A function rather than a bare string so the page cannot leave a tag
// permanently present by assignment, which no physical setup does.
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
	}
	js.Global().Set("shNFC", js.ValueOf(api))
}
