package internal_transitive_external

import (
	"os"
	"testing"
	"time"

	"v.io/x/ref/lib/modules"
	"v.io/x/ref/lib/testutil/expect"
	_ "v.io/x/ref/profiles"

	"v.io/x/devtools/v23/testdata/transitive/middle"
)

var cmd = "moduleInternalOnly"

func init() {
	middle.Init()
}

func Module(t *testing.T) {
	sh, err := modules.NewShell(nil, nil)
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
	s := expect.NewSession(t, m.Stdout(), time.Minute)
	s.Expect(cmd)
}

func TestInternal(t *testing.T) {}
