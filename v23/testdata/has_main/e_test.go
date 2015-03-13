package has_main_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	_ "v.io/x/ref/profiles"
	"v.io/x/ref/test"
	"v.io/x/ref/test/modules"
)

func TestMain(m *testing.M) {
	test.Init()
	if modules.IsModulesChildProcess() {
		if err := modules.Dispatch(); err != nil {
			fmt.Fprintf(os.Stderr, "modules.Dispatch failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}

// Oh..
func moduleHasMainExt(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, "moduleHasMainExt")
	return nil
}

func TestHasMain(t *testing.T) {
	sh, err := modules.NewExpectShell(nil, nil, t, false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start("moduleHasMainExt", nil)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect("moduleHasMainExt")
}
