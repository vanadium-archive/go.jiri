package internal_only

import (
	"fmt"
	"io"
	"os"
	"testing"

	_ "v.io/x/ref/profiles"
	"v.io/x/ref/test/modules"
)

var cmd = "moduleInternalOnly"

// Oh..
func moduleInternalOnly(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, cmd)
	return nil
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
