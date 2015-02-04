package internal_only

import (
	"io"
	"testing"
)

func TestHelperProcess(t *testing.T) {

}

// Oh..
func module(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	return nil
}
