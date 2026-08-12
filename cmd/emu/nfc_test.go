package main

import (
	"io"
	"testing"
)

// The one-shot property is the behaviour a real tag has, and the reason nfc.go
// is untagged: nfc_js.go cannot be tested on this host at all.

func TestATagIsPresentedOnceThenGone(t *testing.T) {
	var n nfcSource
	n.set("md1qqq")
	r := n.reader()
	if r == nil {
		t.Fatal("a presented tag must be readable")
	}
	b, _ := io.ReadAll(r)
	if string(b) != "md1qqq" {
		t.Errorf("read %q, want the presented record", b)
	}
	if n.reader() != nil {
		t.Error("a tag must be readable ONCE — a source that replays forever lets a " +
			"polling flow see a tag that was never presented, which the machine cannot do")
	}
}

func TestNoTagReadsAsNoTag(t *testing.T) {
	var n nfcSource
	if n.reader() != nil {
		t.Error("with nothing presented the reader must be nil, which is the " +
			"supported no-tag value gui already handles")
	}
}

func TestClearRemovesAPendingTag(t *testing.T) {
	var n nfcSource
	n.set("md1qqq")
	n.set("")
	if n.reader() != nil {
		t.Error("clear must remove the pending record")
	}
}

func TestPresentingAgainMakesANewTagReadable(t *testing.T) {
	var n nfcSource
	n.set("first")
	io.ReadAll(n.reader())
	n.set("second")
	r := n.reader()
	if r == nil {
		t.Fatal("presenting a second tag must work")
	}
	b, _ := io.ReadAll(r)
	if string(b) != "second" {
		t.Errorf("read %q, want the newly presented record", b)
	}
}
