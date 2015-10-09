// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"v.io/jiri/runutil"
)

func ExampleSequence() {
	seq := runutil.NewSequence(nil, os.Stdin, ioutil.Discard, ioutil.Discard, false, false, true)
	err := seq.
		Capture(os.Stdout, nil).Run("echo", "a").
		Capture(os.Stdout, nil).Run("echo", "b").
		Done()
	err = seq.
		Run("echo", "c").
		Run("xxxxxxx").
		Capture(os.Stdout, nil).Run("echo", "d").
		Done()
	fmt.Println(err)
	// Output:
	// a
	// b
	// exec: "xxxxxxx": executable file not found in $PATH
}

func TestSequence(t *testing.T) {
	seq := runutil.NewSequence(nil, os.Stdin, os.Stdout, os.Stderr, false, false, true)
	if got, want := seq.Run("echo", "a").Done(), error(nil); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	var out bytes.Buffer
	err := seq.
		Capture(&out, nil).Run("echo", "hello").
		Capture(&out, nil).Run("echo", "world").
		Done()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "hello\nworld\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	out.Reset()
	opts := seq.GetOpts()
	opts.Env = map[string]string{
		"MYTEST":  "hi",
		"MYTEST2": "there",
	}
	err = seq.
		Capture(&out, nil).Opts(opts).Run("sh", "-c", "echo $MYTEST").
		Opts(opts).Capture(&out, nil).Run("sh", "-c", "echo $MYTEST2").
		Done()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "hi\nthere\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	out.Reset()
	err = seq.Run("./bound-to-fail").Done()
	if err == nil {
		t.Fatalf("should have experience an error")
	}
	if got, want := err.Error(), "fork/exec ./bound-to-fail: no such file or directory"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	err = seq.
		Capture(&out, nil).Run("echo", "works, despite previous error").Done()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "works, despite previous error\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	out.Reset()

	err = seq.Timeout(time.Second).Run("sleep", "10").Done()
	if got, want := err.Error(), "command timed out"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

}

// Test that modifiers don't get applied beyond the first invocation of Run.
func TestSequenceModifiers(t *testing.T) {
	seq := runutil.NewSequence(nil, os.Stdin, os.Stdout, os.Stderr, false, false, true)
	var out bytes.Buffer

	opts := seq.GetOpts()
	opts.Env = map[string]string{
		"MYTEST": "hi",
	}
	err := seq.
		Capture(&out, nil).Opts(opts).Run("sh", "-c", "echo $MYTEST").
		Capture(&out, nil).Run("sh", "-c", "echo $MYTEST").
		Done()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "hi\n\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	out.Reset()

	err = seq.
		Capture(&out, nil).Run("echo", "hello").
		Run("echo", "world").
		Done()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "hello\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

type timestamped struct {
	times []time.Time
	data  [][]byte
}

func (t *timestamped) Write(p []byte) (n int, err error) {
	t.times = append(t.times, time.Now())
	t.data = append(t.data, p)
	return len(p), nil
}

func TestSequenceStreaming(t *testing.T) {
	seq := runutil.NewSequence(nil, os.Stdin, os.Stdout, os.Stderr, false, false, true)
	ts := &timestamped{}
	err := seq.
		Capture(ts, nil).Run("sh", "-c", `
	for i in $(seq 1 5); do
		echo $i
		sleep 1
	done`).Done()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(ts.data), 5; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	prev := ts.times[0]
	for _, nth := range ts.times[1:] {
		if nth.Sub(prev) < 500*time.Millisecond {
			t.Errorf("times %s and %s are too close together", nth, prev)
		}
		prev = nth
	}
}

func TestSequenceTermination(t *testing.T) {
	seq := runutil.NewSequence(nil, os.Stdin, os.Stdout, os.Stderr, false, false, true)
	filename := "./test-file"
	fi, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filename)
	opts := seq.GetOpts()
	opts.Stdout = fi
	data, err := seq.Opts(opts).Run("echo", "aha").ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "aha\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TODO(cnicolaou):
// - tests for functions
// - tests for terminating functions, make sure they clean up correctly.
