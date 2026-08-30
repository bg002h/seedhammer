// Package poller implements a NFC device poller for accepting
// data from either tags or writers.
package poller

import (
	"bufio"
	"errors"
	"io"
	"time"

	"seedhammer.com/nfc/ndef"
	"seedhammer.com/nfc/type2"
	"seedhammer.com/nfc/type4"
	"seedhammer.com/nfc/type5"
)

type Device interface {
	Close() error
	Interrupt()
	Detect() (bool, error)
	SetProtocol(prot Protocol) error
	Sleep() error
	ReadCapacity() int
	io.ReadWriter
}

type Poller struct {
	d       Device
	bufr    *bufio.Reader
	emu     *type4.Tag
	reading chan struct{}
	// r is the active reader.
	r io.Reader
}

type Protocol int

const (
	ISO14443a Protocol = iota
	ISO15693
)

func New(d Device) *Poller {
	return &Poller{
		d:       d,
		bufr:    bufio.NewReaderSize(nil, 256),
		emu:     type4.NewTag(d),
		reading: make(chan struct{}, 1),
	}
}

func (p *Poller) Read(buf []byte) (int, error) {
	p.reading <- struct{}{}
	defer func() {
		<-p.reading
	}()
	for {
		if p.r != nil {
			n, err := p.r.Read(buf)
			if err != nil {
				if err != io.EOF || n == 0 {
					p.r = nil
				}
			}
			return n, err
		}
		active, err := p.d.Detect()
		if err != nil {
			return 0, err
		}
		var r io.Reader
		if active {
			// Reset the tag emulator when the
			// external field is off.
			p.emu.Reset()

			r, err = p.poll()
			if err != nil {
				return 0, err
			}
			if r == nil {
				continue
			}
			p.bufr.Reset(r)
			r = ndef.NewMessageReader(p.bufr)
		} else {
			p.bufr.Reset(p.emu)
			r = p.bufr
		}
		p.r = ndef.NewRecordReader(r)
	}
}

// CloseTimeout bounds how long Close waits for an in-flight Read to stop.
//
// Generous on purpose: a healthy read stops as soon as Interrupt lands, so this
// is never reached in normal operation. It exists for the case where it CANNOT
// land, and there the only thing that matters is that the wait ends at all.
const CloseTimeout = 2 * time.Second

// ErrCloseTimeout reports that the in-flight read did not stop, so the device is
// still owned by it and must not be touched.
//
// A caller receiving this must ABANDON the poller and carry on. It must not
// retry, must not block, and must not treat it as fatal: the whole point of the
// error is that continuing with a degraded reader beats freezing.
var ErrCloseTimeout = errors.New("poller: the reader did not stop; abandoning it")

// Close stops the poller. It returns ErrCloseTimeout if the in-flight read could
// not be stopped within CloseTimeout.
//
// F-441: THE WAIT IS BOUNDED, and it was not. This blocked forever on
// `p.reading <- struct{}{}` whenever Interrupt failed to reach the read -- which
// the driver's dropped-signal bug made reachable, and which any future stall in
// the device would make reachable again. The wait runs on the UI goroutine, so
// "forever" meant a frozen panel and a dead machine. The bound is the second
// half of the fix precisely because it does not depend on the first being
// correct: a signal that goes missing for any reason now costs a degraded reader
// instead of the device.
//
// ON TIMEOUT IT DOES NOT TOUCH THE DEVICE. The read still owns the bus, and
// d.Close writes a register; issuing that concurrently would put two writers on
// one I2C transaction. Leaving the chip alone is the safe half of giving up.
func (p *Poller) Close() error {
	select {
	case p.reading <- struct{}{}:
	default:
		p.d.Interrupt()
		t := time.NewTimer(CloseTimeout)
		defer t.Stop()
		select {
		case p.reading <- struct{}{}:
		case <-t.C:
			return ErrCloseTimeout
		}
	}
	return p.d.Close()
}

// poll attempts to select a tag, trying each protocol in turn.
func (p *Poller) poll() (io.Reader, error) {
	if err := p.d.SetProtocol(ISO15693); err != nil {
		return nil, err
	}
	tag15693, err := type5.NewReader(p.d, p.d.ReadCapacity())
	if err == nil {
		return tag15693, nil
	}
	if err := p.d.SetProtocol(ISO14443a); err != nil {
		return nil, err
	}
	tag14443, err := type2.NewReader(p.d)
	if err != nil {
		// Ignore read errors.
		return nil, nil
	}
	return tag14443, nil
}
