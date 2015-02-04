package one_test

import (
	"io"
	"testing"
)

func V23TestB(t *testing.T) { t.FailNow() }

func V23TestC(t *testing.T) { t.FailNow() }

func SubProc3(stdin io.Reader, stdout io.Writer, stderr io.Writer, env map[string]string, args ...string) error {
	return nil
}

func TestMain(t *testing.M) {}

func TestHelperProcess(t *testing.T) {}
