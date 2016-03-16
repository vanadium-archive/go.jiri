// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"v.io/x/lib/envvar"
	"v.io/x/lib/lookpath"
)

const (
	prefix = ">>"
)

type opts struct {
	color   bool
	dir     string
	env     map[string]string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
}

type executor struct {
	indent int
	opts   opts
}

func newExecutor(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, color, verbose bool) *executor {
	if color {
		term := os.Getenv("TERM")
		switch term {
		case "dumb", "":
			color = false
		}
	}
	return &executor{
		indent: 0,
		opts: opts{
			color:   color,
			env:     env,
			stdin:   stdin,
			stdout:  stdout,
			stderr:  stderr,
			verbose: verbose,
		},
	}
}

var (
	commandTimedOutErr = fmt.Errorf("command timed out")
)

// run run's the command and waits for it to finish
func (e *executor) run(timeout time.Duration, opts opts, path string, args ...string) error {
	_, err := e.execute(true, timeout, opts, path, args...)
	return err
}

// start start's the command and does not wait for it to finish.
func (e *executor) start(timeout time.Duration, opts opts, path string, args ...string) (*exec.Cmd, error) {
	return e.execute(false, timeout, opts, path, args...)
}

// function runs the given function and logs its outcome using
// the given options.
func (e *executor) function(opts opts, fn func() error, format string, args ...interface{}) error {
	e.increaseIndent()
	defer e.decreaseIndent()
	e.printf(e.stdoutFromOpts(opts), format, args...)
	err := fn()
	e.printf(e.stdoutFromOpts(opts), okOrFailed(err))
	return err
}

func okOrFailed(err error) string {
	if err != nil {
		return fmt.Sprintf("FAILED: %v", err)
	}
	return "OK"
}

// stdoutFromOpts returns stdout from opts or e.opts if
// in verbose mode.
func (e *executor) stdoutFromOpts(opts opts) io.Writer {
	if opts.verbose && (opts.stdout != nil) {
		return opts.stdout
	}
	if e.opts.verbose && (e.opts.stdout != nil) {
		return e.opts.stdout
	}
	return ioutil.Discard
}

// stderrfromOpts returns stderr from opts or e.opts but
// regardless of verbose mode.
func (e *executor) stderrFromOpts(opts opts) io.Writer {
	if opts.stderr != nil {
		return opts.stderr
	}
	if e.opts.stderr != nil {
		return e.opts.stderr
	}
	return ioutil.Discard
}

// output logs the given list of lines using the given
// options.
func (e *executor) output(opts opts, output []string) {
	if opts.verbose {
		for _, line := range output {
			e.logLine(line)
		}
	}
}

func (e *executor) logLine(line string) {
	if !strings.HasPrefix(line, prefix) {
		e.increaseIndent()
		defer e.decreaseIndent()
	}
	e.printf(e.opts.stdout, "%v", line)
}

// call executes the given Go standard library function,
// encapsulated as a closure.
func (e *executor) call(fn func() error, format string, args ...interface{}) error {
	return e.function(e.opts, fn, format, args...)
}

// execute executes the binary pointed to by the given path using the given
// arguments and options. If the wait flag is set, the function waits for the
// completion of the binary and the timeout value can optionally specify for
// how long should the function wait before timing out.
func (e *executor) execute(wait bool, timeout time.Duration, opts opts, path string, args ...string) (*exec.Cmd, error) {
	e.increaseIndent()
	defer e.decreaseIndent()

	// Check if <path> identifies a binary in the PATH environment
	// variable of the opts.Env.
	if binary, err := lookpath.Look(opts.env, path); err == nil {
		// If so, make sure to execute this binary. This step
		// enables us to "shadow" binaries included in the
		// PATH environment variable of the host OS (which
		// would be otherwise used to lookup <path>).
		//
		// This mechanism is used instead of modifying the
		// PATH environment variable of the host OS as the
		// latter is not thread-safe.
		path = binary
	}
	command := exec.Command(path, args...)
	command.Dir = opts.dir
	command.Stdin = opts.stdin
	command.Stdout = opts.stdout
	command.Stderr = opts.stderr
	command.Env = envvar.MapToSlice(opts.env)
	if opts.verbose || e.opts.verbose {
		args := []string{}
		for _, arg := range command.Args {
			// Quote any arguments that contain '"', ''', '|', or ' '.
			if strings.IndexAny(arg, "\"' |") != -1 {
				args = append(args, strconv.Quote(arg))
			} else {
				args = append(args, arg)
			}
		}
		e.printf(e.stdoutFromOpts(opts), strings.Replace(strings.Join(args, " "), "%", "%%", -1))
	}

	var err error
	switch {
	case !wait:
		err = command.Start()
		e.printf(e.stdoutFromOpts(opts), okOrFailed(err))

	case timeout == 0:
		err = command.Run()
		e.printf(e.stdoutFromOpts(opts), okOrFailed(err))
	default:
		err = e.timedCommand(timeout, opts, command)
		// Verbose output handled in timedCommand.
	}
	return command, err
}

// timedCommand executes the given command, terminating it forcefully
// if it is still running after the given timeout elapses.
func (e *executor) timedCommand(timeout time.Duration, opts opts, command *exec.Cmd) error {
	// Make the process of this command a new process group leader
	// to facilitate clean up of processes that time out.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Kill this process group explicitly when receiving SIGTERM
	// or SIGINT signals.
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
	go func() {
		<-sigchan
		e.terminateProcessGroup(opts, command)
	}()
	if err := command.Start(); err != nil {
		e.printf(e.stdoutFromOpts(opts), "FAILED: %v", err)
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	select {
	case <-time.After(timeout):
		// The command has timed out.
		e.terminateProcessGroup(opts, command)
		// Allow goroutine to exit.
		<-done
		e.printf(e.stdoutFromOpts(opts), "TIMED OUT")
		return commandTimedOutErr
	case err := <-done:
		e.printf(e.stdoutFromOpts(opts), okOrFailed(err))
		return err
	}
}

// terminateProcessGroup sends SIGQUIT followed by SIGKILL to the
// process group (the negative value of the process's pid).
func (e *executor) terminateProcessGroup(opts opts, command *exec.Cmd) {
	pid := -command.Process.Pid
	// Use SIGQUIT in order to get a stack dump of potentially hanging
	// commands.
	if err := syscall.Kill(pid, syscall.SIGQUIT); err != nil {
		e.printf(e.stderrFromOpts(opts), "Kill(%v, %v) failed: %v", pid, syscall.SIGQUIT, err)
	}
	e.printf(e.stderrFromOpts(opts), "Waiting for command to exit: %q", command.Args)
	// Give the process some time to shut down cleanly.
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	// If it still exists, send SIGKILL to it.
	if err := syscall.Kill(pid, 0); err == nil {
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			e.printf(e.stderrFromOpts(opts), "Kill(%v, %v) failed: %v", pid, syscall.SIGKILL, err)
		}
	}
}

func (e *executor) decreaseIndent() {
	e.indent--
}

func (e *executor) increaseIndent() {
	e.indent++
}

func (e *executor) printf(out io.Writer, format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05.00")
	args = append([]interface{}{timestamp, strings.Repeat(prefix, e.indent)}, args...)
	fmt.Fprintf(out, "[%s] %v "+format+"\n", args...)
}
