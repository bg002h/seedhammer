package gui

import "flag"

// update re-records the goldens in this package. Scope it with -run: a bare
// `go test ./... -update` also rewrites backup's sixteen, and those are frozen.
//
// It lives in its own untagged file (moved from freetext_sizeproof_golden_test.go)
// because that file is !refugium and the chain-plate and transaction goldens,
// which run in both builds, read it too.
var update = flag.Bool("update", false, "update golden files")
