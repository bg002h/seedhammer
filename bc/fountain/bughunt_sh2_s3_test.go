package fountain

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md), bug 10.
//
// BCR-2020-005: "`seqLen` is the length of the sequence and `seqNum` and is the
// 1-based index into the sequence." and the message is broken "into `seqLen`
// fragments of fixed size (with the last fragment including padding ...)".

func bhMarshal(t *testing.T, p part) []byte {
	t.Helper()
	enc, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		t.Fatal(err)
	}
	b, err := enc.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func bhAdd(d *Decoder, b []byte) (err error, panicked any) {
	defer func() { panicked = recover() }()
	err = d.Add(b)
	return
}

// Positive control shared by 10a/10b/10d: well-formed parts of a 2-fragment
// message decode to the message.
func bhControl(t *testing.T, msg []byte) {
	t.Helper()
	d := new(Decoder)
	for sn := 1; sn <= 2; sn++ {
		if err, p := bhAdd(d, Encode(msg, sn, 2)); err != nil || p != nil {
			t.Fatalf("control part %d: err=%v panic=%v", sn, err, p)
		}
	}
	got, err := d.Result()
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("control Result = %q, %v", got, err)
	}
}

// Bug 10a: SeqLen=0 panics (index out of range) instead of returning an error.
func TestBughuntSh210aSeqLenZero(t *testing.T) {
	msg := []byte("hello fountain world!!")
	bhControl(t, msg)
	bad := bhMarshal(t, part{
		SeqNum:     1,
		partHeader: partHeader{SeqLen: 0, MessageLen: len(msg), Checksum: Checksum(msg)},
		Data:       msg[:4],
	})
	err, p := bhAdd(new(Decoder), bad)
	if p != nil {
		t.Errorf("Add panicked on SeqLen=0: %v", p)
	} else if err == nil {
		t.Errorf("Add accepted SeqLen=0")
	}
}

// Bug 10b: fragments of different Data lengths panic in reducePart.
func TestBughuntSh210bDataLengthMismatch(t *testing.T) {
	msg := []byte("hello fountain world!!")
	bhControl(t, msg)
	const seqLen = 2
	sum := Checksum(msg)
	hdr := partHeader{SeqLen: seqLen, MessageLen: 8, Checksum: sum} // fragments are 4 bytes
	mixedSeq := SeqNumFor(seqLen, sum, []int{0, 1})
	short := bhMarshal(t, part{SeqNum: 1, partHeader: hdr, Data: []byte{0x01}})                      // fragment 0, 1 byte
	mixed := bhMarshal(t, part{SeqNum: uint32(mixedSeq), partHeader: hdr, Data: []byte{1, 2, 3, 4}}) // fragments 0^1, 4 bytes
	d := new(Decoder)
	var errs []error
	for i, b := range [][]byte{short, mixed} {
		err, p := bhAdd(d, b)
		if p != nil {
			t.Errorf("Add #%d panicked: %v", i+1, p)
			return
		}
		errs = append(errs, err)
	}
	if errs[0] == nil && errs[1] == nil {
		t.Errorf("fragments of 1 and 4 bytes for a 4-byte fragment size were both accepted")
	}
}

const bhChild = "BUGHUNT_SH2_S3_FOUNTAIN_CHILD"

// Bug 10c: SeqLen=1<<31 with SeqNum>SeqLen makes chooseFragments allocate
// SeqLen-sized slices (tens of GB) and the runtime dies out of memory. The
// attempt is made in a child process under RLIMIT_AS so the test run is safe.
func TestBughuntSh210cHugeSeqLen(t *testing.T) {
	if os.Getenv(bhChild) == "1" {
		bhHugeSeqLenChild()
		return
	}
	if runtime.GOOS != "linux" {
		t.Skip("RLIMIT_AS child needs linux")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBughuntSh210cHugeSeqLen$", "-test.v")
	cmd.Env = append(os.Environ(), bhChild+"=1")
	out, err := cmd.CombinedOutput()
	s := string(out)
	if len(s) > 600 {
		s = s[:600] + "..."
	}
	if err != nil || !strings.Contains(string(out), "BHCHILD-SURVIVED") {
		t.Errorf("Add with SeqLen=1<<31 did not return: %v\nchild output: %s", err, s)
	}
}

func bhHugeSeqLenChild() {
	// Cap address space at current size + 2 GiB: the 16 GB allocation cannot fit.
	vm := uint64(0)
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "VmSize:") {
				var kb uint64
				fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(l, "VmSize:")), "%d", &kb)
				vm = kb << 10
			}
		}
	}
	lim := syscall.Rlimit{Cur: vm + 2<<30, Max: vm + 2<<30}
	if err := syscall.Setrlimit(syscall.RLIMIT_AS, &lim); err != nil {
		fmt.Println("BHCHILD-SETRLIMIT-FAILED", err)
		os.Exit(3)
	}
	msg := []byte("hello fountain world!!")
	enc, _ := cbor.CoreDetEncOptions().EncMode()
	bad, _ := enc.Marshal(part{
		SeqNum:     1<<31 + 1,
		partHeader: partHeader{SeqLen: 1 << 31, MessageLen: len(msg), Checksum: Checksum(msg)},
		Data:       msg[:4],
	})
	err := new(Decoder).Add(bad)
	fmt.Printf("BHCHILD-SURVIVED Add returned: %v\n", err)
}

// Bug 10d: SeqNum=0 is accepted; the fragment index becomes 2^32-1, so the
// decoder collects a bogus fragment and never completes.
func TestBughuntSh210dSeqNumZero(t *testing.T) {
	msg := []byte("hello fountain world!!")
	bhControl(t, msg)
	var p part
	if err := cbor.Unmarshal(Encode(msg, 1, 2), &p); err != nil {
		t.Fatal(err)
	}
	p.SeqNum = 0
	bogus := bhMarshal(t, p)
	d := new(Decoder)
	errBogus, pan := bhAdd(d, bogus)
	if pan != nil {
		t.Fatalf("Add panicked on SeqNum=0: %v", pan)
	}
	if errBogus == nil {
		t.Errorf("SeqNum=0 accepted (BCR-2020-005: seqNum is the 1-based index)")
	}
	// Whatever the bogus part did, the two real parts must still decode.
	for sn := 1; sn <= 2; sn++ {
		if err := d.Add(Encode(msg, sn, 2)); err != nil {
			t.Fatalf("real part %d: %v", sn, err)
		}
	}
	got, err := d.Result()
	if err != nil || !bytes.Equal(got, msg) {
		t.Errorf("after the SeqNum=0 part and both real parts, Result = %q, err = %v (completed fragments: %d of SeqLen 2)", got, err, len(d.completed))
	}
}
