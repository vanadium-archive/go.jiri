// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const timedCommandTimeout = 3 * time.Second

var forever time.Duration

func removeTimestamps(t *testing.T, buffer *bytes.Buffer) string {
	result := ""
	scanner := bufio.NewScanner(buffer)
	for scanner.Scan() {
		line := scanner.Text()
		if index := strings.Index(line, prefix); index != -1 {
			result += line[index:] + "\n"
		} else {
			result += line + "\n"
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	return result
}

func TestCommandOK(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	if err := e.run(forever, e.opts, "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandFail(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	if err := e.run(forever, e.opts, "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Command("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	e := newExecutor(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	opts := e.opts
	opts.stdout = &cmdOut
	if err := e.run(forever, opts, "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`CommandWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &runOut), ">> go run ./testdata/ok_hello.go\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestCommandWithOptsFail(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	e := newExecutor(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	opts := e.opts
	opts.stdout = &cmdOut
	if err := e.run(forever, opts, "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`CommandWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &runOut), ">> go run ./testdata/fail_hello.go\n>> FAILED\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestTimedCommandOK(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	if err := e.run(10*time.Second, e.opts, "go", "run", "./testdata/fast_hello.go"); err != nil {
		t.Fatalf(`TimedCommand("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fast_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandFail(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	bin, err := buildTestProgram(e, "slow_hello")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := e.run(timedCommandTimeout, e.opts, bin); err == nil {
		t.Fatalf(`TimedCommand("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := IsTimeout(err), true; got != want {
		t.Fatalf("unexpected error: got %v, want %v", got, want)
	}
	if got, want := removeTimestamps(t, &out), fmt.Sprintf(">> %s\nhello\n>> TIMED OUT\n", bin); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	e := newExecutor(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	opts := e.opts
	opts.stdout = &cmdOut
	if err := e.run(10*time.Second, opts, "go", "run", "./testdata/fast_hello.go"); err != nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &runOut), ">> go run ./testdata/fast_hello.go\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestTimedCommandWithOptsFail(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	e := newExecutor(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	bin, err := buildTestProgram(e, "slow_hello")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	opts := e.opts
	opts.stdout = &cmdOut
	if err := e.run(timedCommandTimeout, opts, bin); err == nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := IsTimeout(err), true; got != want {
		t.Fatalf("unexpected error: got %v, want %v", got, want)
	}
	if got, want := removeTimestamps(t, &runOut), fmt.Sprintf(">> %s\n>> TIMED OUT\n", bin); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestFunctionOK(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := e.function(e.opts, fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Function("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionFail(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("the function failed")
		}
		return nil
	}
	if err := e.function(e.opts, fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Function("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED: the function failed\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsOK(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, false)
	opts := e.opts
	opts.verbose = true
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("the function failed")
		}
		return nil
	}
	if err := e.function(opts, fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsFail(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, false)
	opts := e.opts
	opts.verbose = true
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("the function failed")
		}
		return nil
	}
	if err := e.function(opts, fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED: the function failed\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutput(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	e.output(e.opts, []string{"hello", "world"})
	if got, want := removeTimestamps(t, &out), ">> hello\n>> world\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutputWithOpts(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, false)
	opts := e.opts
	opts.verbose = true
	e.output(opts, []string{"hello", "world"})
	if got, want := removeTimestamps(t, &out), ">> hello\n>> world\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestNested(t *testing.T) {
	var out bytes.Buffer
	e := newExecutor(nil, os.Stdin, &out, ioutil.Discard, false, true)
	fn := func() error {
		e.output(e.opts, []string{"hello", "world"})
		return nil
	}
	e.function(e.opts, fn, "%v", "greetings")
	if got, want := removeTimestamps(t, &out), ">> greetings\n>>>> hello\n>>>> world\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

/*
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
*/

func buildTestProgram(e *executor, fileName string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "runtest")
	if err != nil {
		return "", fmt.Errorf("TempDir() failed: %v", err)
	}
	bin := filepath.Join(tmpDir, fileName)
	buildArgs := []string{"build", "-o", bin, fmt.Sprintf("./testdata/%s.go", fileName)}
	opts := e.opts
	opts.verbose = false
	if err := e.run(forever, opts, "go", buildArgs...); err != nil {
		return "", err
	}
	return bin, nil
}
