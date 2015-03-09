package external_only_test

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"v.io/x/ref/lib/modules"
	"v.io/x/ref/lib/testutil/expect"
	_ "v.io/x/ref/profiles"
)

// Oh..
func moduleExternalOnly(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintf(stdout, "moduleExternalOnly\n")
	return nil
}

func TestExternalOnly(t *testing.T) {
	sh, err := modules.NewShell(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start("moduleExternalOnly", nil)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	s := expect.NewSession(t, m.Stdout(), time.Minute)
	s.Expect("moduleExternalOnly")
}
