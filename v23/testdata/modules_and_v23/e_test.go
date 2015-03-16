package modules_and_v23_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	"v.io/x/ref/test/modules"
	"v.io/x/ref/test/v23tests"
)

func V23TestModulesAndV23A(i *v23tests.T) {}

func V23TestModulesAndV23B(i *v23tests.T) {}

var cmd = "modulesModulesAndV23Ext"

func modulesModulesAndV23Ext(stdin io.Reader, stdout io.Writer, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, cmd)
	return nil
}

func TestModulesAndV23Ext(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
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
