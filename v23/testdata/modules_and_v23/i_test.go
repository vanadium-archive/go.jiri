package modules_and_v23

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"v.io/core/veyron/lib/expect"
	"v.io/core/veyron/lib/modules"
	_ "v.io/core/veyron/profiles"
)

var cmd = "modulesModulesAndV23Int"

// Oh..
func modulesModulesAndV23Int(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, cmd)
	return nil
}

func TestModulesAndV23Int(t *testing.T) {
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
