// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestStartCommandOK tests start.Command() returns immediately without waiting
// for the command to complete.
func TestStartCommandOK(t *testing.T) {
	start := NewStart(nil, os.Stdin, ioutil.Discard, ioutil.Discard, false, false, true)
	bin, err := buildTestProgram(NewRun(nil, os.Stdin, ioutil.Discard, ioutil.Discard, false, false, true), "slow_hello2")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	cmd, err := start.Command(bin)
	if err != nil {
		t.Fatalf(`Command("go run ./testdata/slow_hello2.go") failed to start: %v`, err)
	}
	pid := cmd.Process.Pid
	// Wait a sec and check that the child process is still around.
	time.Sleep(time.Second)
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf(`Command("go run ./testdata/slow_hello2.go") already exited`)
	}
	// We're satisfied.  Go ahead and kill the child to avoid leaving it
	// running after the test completes.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf(`Command("go run ./testdata/slow_hello2.go") couldn't be killed`)
	}
}

func TestStartCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	start := NewStart(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := start.Opts()
	opts.Stdout = &cmdOut
	cmd, err := start.CommandWithOpts(opts, "go", "run", "./testdata/ok_hello.go")
	if err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed to start: %v`, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &cmdOut), "hello\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
