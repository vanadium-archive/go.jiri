// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStartCommandOK tests start.Command() returns immediately without waiting
// for the command to complete.
func TestStartCommandOK(t *testing.T) {
	var out bytes.Buffer
	start := NewStart(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	bin, err := buildSlowHello(NewRun(nil, os.Stdin, &out, ioutil.Discard, false, false, true))
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := start.Command(bin); err != nil {
		t.Fatalf(`Command("go run ./testdata/slow_hello2.go") failed: %v`, err)
	}
	// Note that the output shouldn't have "hello" because start.Command won't
	// wait for the command to finish.
	if got, want := removeTimestamps(t, &out), fmt.Sprintf(">> %s\n>> OK\n", bin); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

// TestStartCommandWithNonZeroExitCodeOK tests that start.Command() succeeds
// even if the command it started failed.
func TestStartCommandWithNonZeroExitCodeOK(t *testing.T) {
	var out bytes.Buffer
	start := NewStart(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if _, err := start.Command("go", "run", "./testdata/fail_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/fail_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestStartCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	start := NewStart(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := start.Opts()
	opts.Stdout = &cmdOut
	if _, err := start.CommandWithOpts(opts, "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	time.Sleep(time.Second * 3)
	if got, want := removeTimestamps(t, &cmdOut), "hello\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
