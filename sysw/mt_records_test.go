package sysw

import (
	"reflect"
	"testing"
)

// Fixtures shared with package mt's tests: the pinned "even" vector (a real
// signed 222-byte tx, set 0x2dcf2), the smuggled-entropy string and the
// foreign-set-id set, all produced by the independent Python encoder. The
// expectations here are the RUST primary's answers (me-cli sysw::mt tests).
var mtEven = []string{
	"mt1p9h8jqq9qqqqgqqqqqqqyqherdfykhhpey6z2cvafak8804qd7g0dl6v8ex9wr2cvky023skwkeud2229sax",
	"mt1p9h8jqq9qqphgdqqqqqqqq0mllllupyqj6vqqqqqqqqzcqpfsw7ph2rt5w54kt768636cls8zxg0najlzunp",
	"mt1p9h8jqq9qqzj8yqpnzw4vl2rwffqyqqqqqkqq282yyhc2vavd20hvk94pz39hts3u5s9a0qd8pwskxfl7ju5",
	"mt1p9h8jqq9qqrqfrnq3qzyp77h37cnxzvwutegzmzy5zrrrfvrpykdfsckvk03dcq6rcjtvlsfcglv7zx43yaz",
	"mt1p9h8jqq9qqylgpzqmhcwhuupdvnrc82rncvzzdahpgjsdwgu52jd7vmxsve9x3w5ujeqyssuvddxvwqze4ve",
	"mt1p9h8jqq9qq9qdcc7h75twfxyf340c4sgqzhfdq6xtgt7zhxngpwa049l0z59l6jqcqqqqqq5k5y2ye5nv8yf",
}

const mtSmuggled = "mt1pm6kmqqqqqq4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w4sqxxtg7uwrnug7"

const evenTxRecord = "tx:020000000001017c8da925af70e49a12b0cea7b639df5037c87b7fa61f262b86ac32c47aa3ba1a0000000000fdffffff02404b4c0000000000160014c1de0dd435d1d4ad97ed1f51d63f91c800cc4eab3ea1b92901000000160014751097c299d6354fbb2c5a84512dd708f2902f5e0247304402207debc7d89984c7717940b622504318d2c184966a618b32cf8b700d0f125b3ffa02206ef875f9c0b5931e0ea1cf0c109bdb8512835c8e51526f99b3419929a2ea7259012103718f5fd45b926226357e2b0400574b41a32d0bf0ae69a02eebea5fbc542ff52060000000"

func TestClassifyMtAndTxRecordsMatchesTheRustPrimary(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  Class
	}{
		{"mt1 plain", mtEven[0], ClassMt},
		{"mt1 leading space", " " + mtEven[0], ClassMt},
		{"mt1 damaged (no correction)", mtEven[0][:len(mtEven[0])-1] + "y", ClassUnknown},
		{"mt1 mixed case", "M" + mtEven[0][1:], ClassUnknown},
		{"tx: real transaction", evenTxRecord, ClassTx},
		{"tx: hex, not a transaction", "tx:abab", ClassUnknown},
		{"tx: not hex", "tx:zz", ClassUnknown},
		{"tx: uppercase hex refused", "tx:ABAB", ClassUnknown},
	}
	for _, c := range cases {
		if got := Classify(c.input); got != c.want {
			t.Errorf("%s: Classify = %v, want %v (Rust's answer)", c.name, got, c.want)
		}
	}
	if ClassMt.IsSecret() || ClassTx.IsSecret() {
		t.Error("mt records are engraved in cleartext; the class must not claim secrecy")
	}
}

func TestMTUnconfirmedMatchesTheRustPrimary(t *testing.T) {
	seed := "abandon abandon abandon abandon abandon abandon " +
		"abandon abandon abandon abandon abandon about"
	cases := []struct {
		name    string
		records []string
		want    []int
	}{
		{"complete set confirms", mtEven, nil},
		{"one chunk missing marks every member", mtEven[:5], []int{0, 1, 2, 3, 4}},
		{"a single chunk alone", mtEven[:1], []int{0}},
		// The semantic arm: complete, BCH-valid, reassembles -- and is 32
		// bytes of entropy, not a transaction.
		{"smuggled entropy", []string{mtSmuggled}, []int{0}},
		// Indices are into the CALLER'S slice, whatever else it holds.
		{"caller indices", []string{seed, mtEven[0]}, []int{1}},
		// Other classes are never reported; the tx: record needs no walk.
		{"other classes untouched", []string{seed, evenTxRecord}, nil},
	}
	for _, c := range cases {
		got := MTUnconfirmed(c.records)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: MTUnconfirmed = %v, want %v", c.name, got, c.want)
		}
	}
	// Two sets grouped apart: the complete one confirms beside a stranger.
	mixed := append(append([]string{}, mtEven...), mtSmuggled)
	if got := MTUnconfirmed(mixed); !reflect.DeepEqual(got, []int{6}) {
		t.Errorf("grouping: got %v, want [6]", got)
	}
}
