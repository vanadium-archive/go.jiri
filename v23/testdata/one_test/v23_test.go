// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package one_test

import "v.io/core/veyron/lib/modules"
import "v.io/core/veyron/lib/testutil/v23tests"

func init() {
	modules.RegisterChild("SubProc3", ``, SubProc3)
}

func TestV23B(t *testing.T) {
	v23tests.RunTest(t, V23TestB)
}

func TestV23C(t *testing.T) {
	v23tests.RunTest(t, V23TestC)
}
