// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil_test

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

	"v.io/jiri/runutil"
	"v.io/x/lib/envvar"
	"v.io/x/lib/lookpath"
)

const timedCommandTimeout = 3 * time.Second

var forever time.Duration

func removeTimestamps(t *testing.T, buffer *bytes.Buffer) string {
	result := ""
	scanner := bufio.NewScanner(buffer)
	for scanner.Scan() {
		line := scanner.Text()
		if index := strings.Index(line, ">>"); index != -1 {
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

func osPath(t *testing.T, bin string) string {
	path, err := lookpath.Look(envvar.SliceToMap(os.Environ()), bin)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func removeTimestampsAndPath(t *testing.T, buffer *bytes.Buffer, bin string) string {
	s := removeTimestamps(t, buffer)
	return strings.Replace(s, osPath(t, bin), bin, 1)
}

func TestCommandOK(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	if err := s.Last("go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestampsAndPath(t, &out, "go"), ">> go run ./testdata/ok_hello.go\n>> OK\nhello\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	var cout bytes.Buffer
	if err := s.Capture(&cout, nil).Last("go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := cout.String(), "hello\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandFail(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	if err := s.Last("go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Command("go run ./testdata/fail_hello.go") did not fail when it should`)
	}

	if got, wantCommon, want1, want2 := removeTimestampsAndPath(t, &out, "go"), ">> go run ./testdata/fail_hello.go\n>> FAILED: exit status 1\n", "hello\nexit status 1\n", "exit status 1\nhello\n"; got != wantCommon+want1 && got != wantCommon+want2 {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\nor\n%v\n", got, wantCommon+want1, wantCommon+want2)
	}
	var cout, cerr bytes.Buffer
	if err := s.Capture(&cout, &cerr).Last("go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Command("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := cout.String(), "hello\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := cerr.String(), "exit status 1\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	if err := s.Capture(&cmdOut, nil).Verbose(false).Last("go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`CommandWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestampsAndPath(t, &runOut, "go"), ">> go run ./testdata/ok_hello.go\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestCommandWithOptsFail(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	if err := s.Capture(&cmdOut, nil).Verbose(false).Last("go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`CommandWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestampsAndPath(t, &runOut, "go"), ">> go run ./testdata/fail_hello.go\n>> FAILED: exit status 1\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestTimedCommandOK(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	bin, err := buildTestProgram("fast_hello")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Timeout(2 * time.Minute).Last(bin); err != nil {
		t.Fatalf(`TimedCommand("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), fmt.Sprintf(">> %s\n>> OK\nhello\n", bin); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandFail(t *testing.T) {
	var out, stderr bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, &stderr, false, true)
	bin, err := buildTestProgram("slow_hello")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Timeout(timedCommandTimeout).Last(bin); err == nil {
		t.Fatalf(`TimedCommand("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := runutil.IsTimeout(err), true; got != want {
		t.Fatalf("unexpected error: got %v, want %v", got, want)
	}
	o := removeTimestamps(t, &out)
	o = strings.Replace(o, bin, "slow_hello", 1)
	if got, want := o, `>> slow_hello
>> TIMED OUT
hello
>> Waiting for command to exit: ["`+bin+`"]
`; !strings.HasPrefix(o, want) {
		t.Errorf("output doesn't start with %v, got: %v (stderr: %v)", want, got, stderr.String())
	}
}

func TestTimedCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	bin, err := buildTestProgram("fast_hello")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}

	if err := s.Timeout(2*time.Minute).Verbose(false).Capture(&cmdOut, nil).Last(bin); err != nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &runOut), fmt.Sprintf(">> %s\n>> OK\n", bin); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
	if got, want := strings.TrimSpace(cmdOut.String()), "hello"; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func TestTimedCommandWithOptsFail(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &runOut, ioutil.Discard, false, true)
	bin, err := buildTestProgram("slow_hello")
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.Timeout(timedCommandTimeout).Capture(&cmdOut, nil).Verbose(false).Last(bin); err == nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := runutil.IsTimeout(err), true; got != want {
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
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := s.Capture(&out, nil).Call(fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go").Done(); err != nil {
		t.Fatalf(`Function("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionFail(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("the function failed")
		}

		return nil
	}
	if err := s.Capture(&out, nil).Call(fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go").Done(); err == nil {
		t.Fatalf(`Function("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED: the function failed\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsOK(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, false)

	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("the function failed")
		}
		return nil
	}
	if err := s.Capture(&out, nil).Verbose(true).Call(fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go").Done(); err != nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsFail(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, false)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("the function failed")
		}
		return nil
	}
	if err := s.Capture(&out, nil).Verbose(true).Call(fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go").Done(); err == nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED: the function failed\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutput(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	s.Output([]string{"hello", "world"})
	if got, want := removeTimestamps(t, &out), ">> hello\n>> world\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutputWithOpts(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, false)
	s.Verbose(true).Output([]string{"hello", "world"})
	if got, want := removeTimestamps(t, &out), ">> hello\n>> world\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestNested(t *testing.T) {
	var out bytes.Buffer
	s := runutil.NewSequence(nil, os.Stdin, &out, ioutil.Discard, false, true)
	fn := func() error {
		s.Output([]string{"hello", "world"})
		return nil
	}
	s.Call(fn, "%v", "greetings").Done()
	if got, want := removeTimestamps(t, &out), ">> greetings\n>>>> hello\n>>>> world\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func buildTestProgram(fileName string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "runtest")
	if err != nil {
		return "", fmt.Errorf("TempDir() failed: %v", err)
	}
	bin := filepath.Join(tmpDir, fileName)
	buildArgs := []string{"build", "-o", bin, fmt.Sprintf("./testdata/%s.go", fileName)}
	s := runutil.NewSequence(nil, os.Stdin, os.Stdout, os.Stderr, false, true)
	if err := s.Last("go", buildArgs...); err != nil {
		return "", err
	}
	return bin, nil
}
