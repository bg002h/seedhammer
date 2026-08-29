//go:build tinygo

package gui

// notifyPlateText and notifyPlateEngraved do nothing on the machine, and
// EngravedAware does not exist here.
//
// See engraved_hook.go for why the firmware carries neither: the string these
// would pass is the operator's descriptor, key card or SECRET share, and the
// only consumer for it lives in a browser. An interface the image does not
// contain cannot be implemented by accident.
//
// WHAT IT COSTS, MEASURED: 1,024 bytes. Built at the production settings
// (-target pico-plus2 -stack-size 16kb -gc precise -opt 2 -scheduler tasks)
// with VERSION pinned so the stamp could not differ:
//
//	before the hook    2,683,904 bytes   sha256 b3e622a8...
//	with the hook      2,684,928 bytes   sha256 7b8633d5...
//
// Two more UF2 blocks, across 530,938 differing bytes -- which at an unchanged
// +1KB is code shifting downstream, not code being added. Unlike frame_hook's
// call, which the compiler removes entirely, this one is not free: the change
// is not only the two calls but the id field on [Plate], a struct the whole
// engrave path passes by value, and the counter that fills it.
//
// AND THE IMAGE IS NOT THE COST THAT MATTERED. The first version built the id
// slice at the call site in validateMdmk, so the DEVICE allocated a []uint64
// per validated string that nothing on the device ever read -- a heap
// allocation on a machine with a 16KB stack and a precise GC, to serve a
// browser. Moving it into engraved_hook.go, which the firmware never compiles,
// removed it. Measured again afterwards: still +1,024 bytes, so the image cost
// is the field and the calls, and the allocation was pure waste that the size
// number alone would never have surfaced.
func notifyPlateText(Platform, []Plate, []string) {}

func notifyPlateEngraved(Platform, uint64) {}
