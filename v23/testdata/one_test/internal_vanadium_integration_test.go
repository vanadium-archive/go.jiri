// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package one

import "testing"

import "v.io/core/veyron/lib/modules"

func init() {
	modules.RegisterChild("SubProc", `SubProc does the following...
Usage: <a> <b>...`, SubProc)
	modules.RegisterChild("SubProc2", `SubProc2 does the following...
<ab> <cd>`, SubProc2)
}

func TestHelperProcess(t *testing.T) {
	modules.DispatchInTest()
}
