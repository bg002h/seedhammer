package ap33772s

import (
	"encoding/binary"
	"testing"
)

// ocpModelBus models the AP33772S registers LimitCurrent touches, and the
// overcurrent-protection (OCP) threshold they produce.
//
// AP33772S datasheet, "Overcurrent Protection (OCP) Threshold Setting": "The
// user defined OCPTHR value is an 8-bit register with 50mA/LSB and 00h by
// default. After successful negotiation with the PD source, if the OCPTHR
// value is still 00h, the OCP Threshold Current would be 110% of the IMAX,
// maximum current of the selected PDO/APDO. If the OCPTHR value has been
// updated through I2C command, the OCP Threshold Current would be 110% of
// IOCPTHR value." (Table 7: OCPTHR = 0 -> IMAX*110%; OCPTHR != 0 ->
// IOCPTHR*110%.)
type ocpModelBus struct {
	imaxmA  int // Maximum current of the negotiated PDO.
	ocpthr  uint8
	written bool
}

func (b *ocpModelBus) Tx(addr uint16, w, r []byte) error {
	if r != nil {
		// Only IREQ (10 mA/LSB, 16 bits) is read by LimitCurrent.
		binary.LittleEndian.PutUint16(r, uint16(b.imaxmA/10))
		return nil
	}
	if len(w) == 2 && w[0] == regOCPTHR {
		b.ocpthr = w[1]
		b.written = true
	}
	return nil
}

// thresholdmA is the OCP threshold current from datasheet Table 7.
func (b *ocpModelBus) thresholdmA() int {
	if b.ocpthr == 0 {
		return b.imaxmA * 110 / 100
	}
	return int(b.ocpthr) * 50 * 110 / 100
}

// TestBughuntSh240LimitCurrentZero is bug #40: the report says that
// LimitCurrent(0) writes 0 to OCPTHR "which can trip overcurrent protection or
// cut power". The datasheet (Table 7) says OCPTHR = 0 is the default and
// selects IMAX*110%, which is what LimitCurrent's doc comment promises ("A
// limit of zero represents the negotiated limit"). The test asserts the
// effect: a zero limit leaves the threshold at the negotiated current.
func TestBughuntSh240LimitCurrentZero(t *testing.T) {
	const imaxmA = 3000
	t.Run("zero-means-negotiated", func(t *testing.T) {
		b := &ocpModelBus{imaxmA: imaxmA}
		if err := New(b).LimitCurrent(0); err != nil {
			t.Fatal(err)
		}
		if got, want := b.thresholdmA(), imaxmA*110/100; got < want {
			t.Errorf("OCP threshold %d mA after LimitCurrent(0), want %d mA (IMAX*110%%)", got, want)
		}
	})
	t.Run("control-1500", func(t *testing.T) {
		b := &ocpModelBus{imaxmA: imaxmA}
		if err := New(b).LimitCurrent(1500); err != nil {
			t.Fatal(err)
		}
		if got, want := b.thresholdmA(), 1500*110/100; got != want {
			t.Errorf("OCP threshold %d mA after LimitCurrent(1500), want %d mA", got, want)
		}
	})
}
