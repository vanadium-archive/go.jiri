// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStartCommandOK tests start.Command() returns immediately without waiting
// for the command to complete.
func TestStartCommandOK(t *testing.T) {
	var out bytes.Buffer
	start := NewStart(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	bin, err := buildTestProgram(NewRun(nil, os.Stdin, &out, ioutil.Discard, false, false, true), "slow_hello2")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := start.Command(bin); err != nil {
		t.Fatalf(`Command("go run ./testdata/slow_hello2.go") failed: %v`, err)
	}
	// Note that the output shouldn't have "hello!!" because start.Command won't
	// wait for the command to finish.
	output := removeTimestamps(t, &out)
	if strings.Index(output, "hello!!") != -1 {
		t.Fatalf("output shouldn't contain 'hello!!':\n%v", output)
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
