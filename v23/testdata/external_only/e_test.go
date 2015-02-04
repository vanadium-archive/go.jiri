package external_only_test

import "io"
import "testing"

func TestMain(t *testing.M) {}

// Oh..
func module(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	return nil
}
