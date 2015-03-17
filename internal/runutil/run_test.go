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
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.Command("go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Command("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.Command("go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Command("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.CommandWithOpts(opts, "go", "run", "./testdata/ok_hello.go"); err != nil {
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
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.CommandWithOpts(opts, "go", "run", "./testdata/fail_hello.go"); err == nil {
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
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	if err := run.TimedCommand(10*time.Second, "go", "run", "./testdata/fast_hello.go"); err != nil {
		t.Fatalf(`TimedCommand("go run ./testdata/fast_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fast_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	bin, err := buildSlowHello(run)
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := run.TimedCommand(timedCommandTimeout, bin); err == nil {
		t.Fatalf(`TimedCommand("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := err, CommandTimedOutErr; got != want {
		t.Fatalf("unexpected error: got %v, want %v", got, want)
	}
	if got, want := removeTimestamps(t, &out), fmt.Sprintf(">> %s\nhello\n>> TIMED OUT\n", bin); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTimedCommandWithOptsOK(t *testing.T) {
	var cmdOut, runOut bytes.Buffer
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.TimedCommandWithOpts(10*time.Second, opts, "go", "run", "./testdata/fast_hello.go"); err != nil {
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
	run := New(nil, os.Stdin, &runOut, ioutil.Discard, false, false, true)
	bin, err := buildSlowHello(run)
	if bin != "" {
		defer os.RemoveAll(filepath.Dir(bin))
	}
	if err != nil {
		t.Fatalf("%v", err)
	}
	opts := run.Opts()
	opts.Stdout = &cmdOut
	if err := run.TimedCommandWithOpts(timedCommandTimeout, opts, bin); err == nil {
		t.Fatalf(`TimedCommandWithOpts("go run ./testdata/slow_hello.go") did not fail when it should`)
	} else if got, want := err, CommandTimedOutErr; got != want {
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
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.Function(fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`Function("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.Function(fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`Function("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsOK(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, false)
	opts := run.Opts()
	opts.Verbose = true
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/ok_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.FunctionWithOpts(opts, fn, "%v %v %v", "go", "run", "./testdata/ok_hello.go"); err != nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/ok_hello.go") failed: %v`, err)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/ok_hello.go\nhello\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestFunctionWithOptsFail(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, false)
	opts := run.Opts()
	opts.Verbose = true
	fn := func() error {
		cmd := exec.Command("go", "run", "./testdata/fail_hello.go")
		cmd.Stdout = &out
		return cmd.Run()
	}
	if err := run.FunctionWithOpts(opts, fn, "%v %v %v", "go", "run", "./testdata/fail_hello.go"); err == nil {
		t.Fatalf(`FunctionWithOpts("go run ./testdata/fail_hello.go") did not fail when it should`)
	}
	if got, want := removeTimestamps(t, &out), ">> go run ./testdata/fail_hello.go\nhello\n>> FAILED\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutput(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	run.Output([]string{"hello", "world"})
	if got, want := removeTimestamps(t, &out), ">> hello\n>> world\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestOutputWithOpts(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, false)
	opts := run.Opts()
	opts.Verbose = true
	run.OutputWithOpts(opts, []string{"hello", "world"})
	if got, want := removeTimestamps(t, &out), ">> hello\n>> world\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestNested(t *testing.T) {
	var out bytes.Buffer
	run := New(nil, os.Stdin, &out, ioutil.Discard, false, false, true)
	fn := func() error {
		run.Output([]string{"hello", "world"})
		return nil
	}
	run.Function(fn, "%v", "greetings")
	if got, want := removeTimestamps(t, &out), ">> greetings\n>>>> hello\n>>>> world\n>> OK\n"; got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func buildSlowHello(run *Run) (string, error) {
	tmpDir, err := ioutil.TempDir("", "runtest")
	if err != nil {
		return "", fmt.Errorf("TempDir() failed: %v", err)
	}
	bin := filepath.Join(tmpDir, "slow_hello")
	buildArgs := []string{"build", "-o", bin, "./testdata/slow_hello.go"}
	opts := run.Opts()
	opts.Verbose = false
	if err := run.CommandWithOpts(opts, "go", buildArgs...); err != nil {
		return "", err
	}
	return bin, nil
}
