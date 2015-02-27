// The following enables go generate to generate the doc.go file.
//
//go:generate go run $VANADIUM_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"os"
)

func main() {
	os.Exit(root().Main())
}
