package transitive

import (
	"os"
	"testing"

	_ "v.io/x/ref/profiles"
	"v.io/x/ref/test/modules"

	"v.io/x/devtools/v23/testdata/transitive/middle"
)

var cmd = "moduleInternalOnly"

func init() {
	middle.Init()
}

func TestModulesInternalOnly(t *testing.T) {
	sh, err := modules.NewExpectShell(nil, nil, t, false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(cmd, nil)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect(cmd)
}
