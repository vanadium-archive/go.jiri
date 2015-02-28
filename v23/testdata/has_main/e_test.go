package has_main_test

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"v.io/x/ref/lib/expect"
	"v.io/x/ref/lib/modules"
	"v.io/x/ref/lib/testutil"
	_ "v.io/x/ref/profiles"
)

func TestMain(m *testing.M) {
	testutil.Init()
	if modules.IsModulesProcess() {
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
	sh, err := modules.NewShell(nil, nil)
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
	s := expect.NewSession(t, m.Stdout(), time.Minute)
	s.Expect("moduleHasMainExt")
}
