// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"v.io/jiri/runutil"
)

func rmLineNumbers(s string) string {
	re := regexp.MustCompile("(.*\\.go):\\d+:(.*)")
	return re.ReplaceAllString(s, "$1:-:$2")
}

func sanitizeTimestamps(s string) string {
	re := regexp.MustCompile(`\[(\d\d:\d\d:\d\d.\d\d)\]`)
	return re.ReplaceAllString(s, "[hh:mm:ss.xx]")
}

func ExampleSequence() {
	seq := runutil.NewSequence(nil, os.Stdin, ioutil.Discard, ioutil.Discard, false, false, true)
	err := seq.
		Capture(os.Stdout, nil).Run("echo", "a").
		Capture(os.Stdout, nil).Last("echo", "b")
	err = seq.
		Run("echo", "c").
		Run("xxxxxxx").
		Capture(os.Stdout, nil).Last("echo", "d")
	// Get rid of the line#s in the error output.
	fmt.Println(rmLineNumbers(err.Error()))
	// Output:
	// a
	// b
	// sequence_test.go:-: Run("xxxxxxx"): exec: "xxxxxxx": executable file not found in $PATH
}

// TestStdoutStderr exercises the various possible configurations for stdout and
// stderr (via NewSequence, Opts, or Capture) as well as the verbose flag.
func TestStdoutStderr(t *testing.T) {
	// Case 1: we only specify stdout/stderr at constructor time.
	//
	// Verbose mode: All the command's output and execution logging goes to
	// stdout, execution error messages to stderr.
	//
	// Non-Verbose mode: No stdout output; execution error messages to
	// stderr.
	for _, verbose := range []bool{false, true} {
		var cnstrStdout, cnstrStderr bytes.Buffer
		seq := runutil.NewSequence(nil, os.Stdin, &cnstrStdout, &cnstrStderr, false, false, verbose)
		seq.Run("bash", "-c", "echo a; echo b >&2").
			Timeout(time.Microsecond).
			Run("sleep", "10000")
		want := ""
		if verbose {
			want = `[hh:mm:ss.xx] >> bash -c "echo a; echo b >&2"
[hh:mm:ss.xx] >> OK
a
b
[hh:mm:ss.xx] >> sleep 10000
[hh:mm:ss.xx] >> TIMED OUT
`
		}
		if got := sanitizeTimestamps(cnstrStdout.String()); want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := cnstrStderr.String(), "Waiting for command to exit: [\"sleep\" \"10000\"]\n"; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
	}

	// Case 2: we specify stdout/stderr at constructor time, and also via
	// Opts.  The verbose setting from opts takes precedence and controls
	// the output that goes both to constructor stdout and to opts stdout.
	//
	// Verbose mode: The command execution logging goes to constructor
	// stdout, command execution errors go to constructor stderr, and the
	// actual command output goes to opts stdout. Nothing goes to opts
	// stderr.
	//
	// Non-Verbose mode: No stdout output; execution error messages to
	// constructor stderr.
	for _, verbose := range []bool{false, true} {
		var cnstrStdout, cnstrStderr, optsStdout, optsStderr bytes.Buffer
		cstrVerbose := false // irellevant, the opts verbose flag takes precedence.
		seq := runutil.NewSequence(nil, os.Stdin, &cnstrStdout, &cnstrStderr, false, false, cstrVerbose)
		opts := runutil.Opts{Stdout: &optsStdout, Stderr: &optsStderr, Verbose: verbose}
		seq.Opts(opts).
			Run("bash", "-c", "echo a; echo b >&2").
			Opts(opts).
			Timeout(time.Microsecond).
			Run("sleep", "10000")
		want := ""
		if verbose {
			want = `[hh:mm:ss.xx] >> bash -c "echo a; echo b >&2"
[hh:mm:ss.xx] >> OK
[hh:mm:ss.xx] >> sleep 10000
[hh:mm:ss.xx] >> TIMED OUT
`
		}
		if got := sanitizeTimestamps(cnstrStdout.String()); want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := cnstrStderr.String(), "Waiting for command to exit: [\"sleep\" \"10000\"]\n"; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		want = ""
		if verbose {
			want = "a\nb\n"
		}
		if got := optsStdout.String(); want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := optsStderr.String(), ""; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
	}

	// Case 3: we specify stdout/stderr at constructor time, also via Opts,
	// and also via Capture.  The verbose setting from opts takes
	// precedence.
	//
	// Verbose mode: The command execution log goes to constructor stdout,
	// command execution errors go to constructor stderr, and the
	// stdout/stderr output from the command goes to capture stdout/stderr
	// respectively.  Nothing goes to opts stdout/stderr.
	//
	// Non-Verbose mode: The stdout/stderr output from the command goes to
	// capture stdout/stderr respectively.  No command execution log, but
	// the command execution errors go to constructor stderr.  Nothing goes
	// to opts stdout/stderr.
	for _, verbose := range []bool{false, true} {
		var cnstrStdout, cnstrStderr, optsStdout, optsStderr, captureStdout, captureStderr bytes.Buffer
		cstrVerbose := false // irellevant, the opts verbose flag takes precedence.
		seq := runutil.NewSequence(nil, os.Stdin, &cnstrStdout, &cnstrStderr, false, false, cstrVerbose)
		opts := runutil.Opts{Stdout: &optsStdout, Stderr: &optsStderr, Verbose: verbose}
		seq.Opts(opts).
			Capture(&captureStdout, &captureStderr).
			Run("bash", "-c", "echo a; echo b >&2").
			Opts(opts).
			Timeout(time.Microsecond).
			Run("sleep", "10000")
		want := ""
		if verbose {
			want = `[hh:mm:ss.xx] >> bash -c "echo a; echo b >&2"
[hh:mm:ss.xx] >> OK
[hh:mm:ss.xx] >> sleep 10000
[hh:mm:ss.xx] >> TIMED OUT
`
		}
		if got := sanitizeTimestamps(cnstrStdout.String()); want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := cnstrStderr.String(), "Waiting for command to exit: [\"sleep\" \"10000\"]\n"; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := optsStdout.String(), ""; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := optsStderr.String(), ""; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := captureStdout.String(), "a\n"; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
		if got, want := captureStderr.String(), "b\n"; want != got {
			t.Errorf("verbose: %t, got %v, want %v", verbose, got, want)
		}
	}
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
	err = seq.Run("./bound-to-fail", "fail").Done()
	if err == nil {
		t.Fatalf("should have experience an error")
	}
	if got, want := rmLineNumbers(err.Error()), "sequence_test.go:-: Run(\"./bound-to-fail\", \"fail\"): fork/exec ./bound-to-fail: no such file or directory"; got != want {
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
	if got, want := rmLineNumbers(err.Error()), "sequence_test.go:-: Run(\"sleep\", \"10\"): command timed out"; got != want {
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
		Capture(&out, nil).Last("sh", "-c", "echo $MYTEST")
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

func TestSequenceOutputOnError(t *testing.T) {
	var out bytes.Buffer
	// Only the output from the command that generates an error is written
	// to out when not in verbose mode.
	seq := runutil.NewSequence(nil, os.Stdin, &out, os.Stderr, false, false, false)
	err := seq.Run("sh", "-c", "echo not me").
		Run("sh", "-c", "echo ooh; echo ah; echo me; exit 1").
		Last("sh", "-c", "echo not me either")
	if err == nil {
		t.Errorf("expected an error")
	}
	if got, want := out.String(), "oh\nah"; !strings.Contains(got, want) {
		t.Errorf("got %v doesn't contain %v", got, want)
	}
	if got, notWant := out.String(), "not me either"; strings.Contains(got, notWant) {
		t.Errorf("got %v contains %v", got, notWant)
	}

	out.Reset()
	err = seq.Run("sh", "-c", "echo hard to not include me").
		Run("sh", "-c", "echo ooh; echo ah; echo me").
		Last("sh", "-c", "echo not me either")
	if err != nil {
		t.Error(err)
	}
	if got, want := len(out.String()), 0; got != want {
		t.Logf(out.String())
		t.Errorf("got %v, want %v", got, want)
	}

	out.Reset()
	// All output is written to out when in verbose mode.
	seq = runutil.NewSequence(nil, os.Stdin, &out, os.Stderr, false, false, true)
	err = seq.Run("sh", "-c", "echo AA").
		Run("sh", "-c", "echo BB; exit 1").
		Last("sh", "-c", "echo CC")
	if got, want := strings.Count(out.String(), "\n"), 8; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := strings.Count(out.String(), "AA"), 2; got != want {
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
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "aha\nCurrent Directory: "+cwd+"\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func getwd(t *testing.T) string {
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return here
}

func TestSequencePushPop(t *testing.T) {
	here := getwd(t)
	s := runutil.NewSequence(nil, os.Stdin, os.Stdout, os.Stderr, false, false, true)
	components := []string{here, "test", "a", "b", "c"}
	tree := filepath.Join(components...)
	s.MkdirAll(tree, os.FileMode(0755))
	if err := s.Error(); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Join(here, "test"))

	td := ""
	for _, d := range components {
		s.Pushd(d)
		td = filepath.Join(td, d)
		if got, want := getwd(t), td; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	s.Done()
	if got, want := getwd(t), here; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	s.Pushd("test").Pushd("a").Pushd("b")
	if got, want := getwd(t), filepath.Join(here, "test", "a", "b"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	err := s.Pushd("x").Done()
	if err == nil {
		t.Fatal(fmt.Errorf("expected an error"))
	}
	// Make sure the stack is unwound on error.
	if got, want := getwd(t), here; got != want {
		t.Errorf("got %v, want %v", got, want)
		if err := os.Chdir(here); err != nil {
			panic(fmt.Sprintf("failed to chdir back to %s", here))
		}

	}
}

// TODO(cnicolaou):
// - tests for functions
// - tests for terminating functions, make sure they clean up correctly.
