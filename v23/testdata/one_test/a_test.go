package one

import (
	"io"
	"testing"
)

func TestA(t *testing.T) {}

func V23TestA(t *testing.T) { t.FailNow() }

// SubProc does the following...
// Usage: <a> <b>...
func SubProc(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	return nil
}

// SubProc2 does the following...
// <ab> <cd>
func SubProc2(stdin io.Reader, stdout io.Writer, stderr io.Writer, env map[string]string, args ...string) error {
	return nil
}
