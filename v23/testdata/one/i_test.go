package one

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"v.io/core/veyron/lib/expect"
	"v.io/core/veyron/lib/modules"
	"v.io/core/veyron/lib/testutil/v23tests"
	_ "v.io/core/veyron/profiles"
)

func TestV23TestMain(t *testing.T) {}

// This will not be picked up as a V23 test.
func V23TestOneIgnored(i *v23tests.T) { i.FailNow() }

// modulesOneInt does the following...
// Usage: <a> <b>...
func modulesOneInt(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, "modulesOneInt")
	return nil
}

// modulesTwoInt does the following...
// <ab> <cd>
func modulesTwoInt(stdin io.Reader, stdout io.Writer, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, "modulesTwoInt")
	return nil
}

func TestModulesOneAndTwo(t *testing.T) {
	sh, err := modules.NewShell(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"modulesOneInt", "modulesTwoInt"} {
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
}
