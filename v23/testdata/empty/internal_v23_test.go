// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package empty

import "testing"
import "os"

import "v.io/core/veyron/lib/testutil"

func TestMain(m *testing.M) {
	testutil.Init()
	cleanup := v23tests.UseSharedBinDir()
	r := m.Run()
	cleanup()
	// TODO(cnicolaou): call modules.Dispatch and remove the need for TestHelperProcess
	os.Exit(r)
}
