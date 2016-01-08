// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"v.io/x/lib/envvar"
)

const (
	prefix = ">>"
)

type opts struct {
	color   bool
	dir     string
	dryRun  bool
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

func newExecutor(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, color, dryRun, verbose bool) *executor {
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
			dryRun:  dryRun,
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
	if opts.verbose {
		e.printf(e.opts.stdout, format, args...)
	}
	err := fn()
	if err != nil {
		if opts.verbose {
			e.printf(e.opts.stdout, "FAILED: %v", err)
		}
		return err
	}
	if opts.verbose {
		e.printf(e.opts.stdout, "OK")
	}
	return nil
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
// encapsulated as a closure, respecting the "dry run" option.
func (e *executor) call(fn func() error, format string, args ...interface{}) error {
	if opts := e.opts; opts.dryRun {
		opts.verbose = true
		return e.function(opts, func() error { return nil }, format, args...)
	}
	return e.function(e.opts, fn, format, args...)
}

// alwaysRun executes the given Go standard library function, encapsulated as a
// closure, but translating "dry run" into "verbose" for this particular
// command so that the command can execute and thus allow subsequent
// commands to complete. It is generally used for testing/making files/directories
// that affect subsequent behaviour.
func (e *executor) alwaysRun(fn func() error, format string, args ...interface{}) error {
	if opts := e.opts; opts.dryRun {
		// Disable the dry run option as this function has no effect and
		// doing so results in more informative "dry run" output.
		opts.dryRun = false
		opts.verbose = true
		return e.function(opts, fn, format, args...)
	}
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
	binary, err := LookPath(path, opts.env)
	if err == nil {
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
	if opts.verbose || opts.dryRun {
		args := []string{}
		for _, arg := range command.Args {
			// Quote any arguments that contain '"', ''', '|', or ' '.
			if strings.IndexAny(arg, "\"' |") != -1 {
				args = append(args, strconv.Quote(arg))
			} else {
				args = append(args, arg)
			}
		}
		e.printf(e.opts.stdout, strings.Replace(strings.Join(args, " "), "%", "%%", -1))
	}
	if opts.dryRun {
		e.printf(e.opts.stdout, "OK")
		return nil, nil
	}

	if wait {
		if timeout == 0 {
			if err = command.Run(); err != nil {
				if opts.verbose {
					e.printf(e.opts.stdout, "FAILED")
				}
			} else {
				if opts.verbose {
					e.printf(e.opts.stdout, "OK")
				}
			}
		} else {
			err = e.timedCommand(timeout, opts, command)
		}
	} else {
		err = command.Start()
		if err != nil {
			if opts.verbose {
				e.printf(e.opts.stdout, "FAILED")
			}
		} else {
			if opts.verbose {
				e.printf(e.opts.stdout, "OK")
			}
		}
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
		e.terminateProcessGroup(command)
	}()
	if err := command.Start(); err != nil {
		if opts.verbose {
			e.printf(e.opts.stdout, "FAILED")
		}
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	select {
	case <-time.After(timeout):
		// The command has timed out.
		e.terminateProcessGroup(command)
		// Allow goroutine to exit.
		<-done
		if opts.verbose {
			e.printf(e.opts.stdout, "TIMED OUT")
		}
		return commandTimedOutErr
	case err := <-done:
		if err != nil {
			if opts.verbose {
				e.printf(e.opts.stdout, "FAILED")
			}
		} else {
			if opts.verbose {
				e.printf(e.opts.stdout, "OK")
			}
		}
		return err
	}
}

// terminateProcessGroup sends SIGQUIT followed by SIGKILL to the
// process group (the negative value of the process's pid).
func (e *executor) terminateProcessGroup(command *exec.Cmd) {
	pid := -command.Process.Pid
	// Use SIGQUIT in order to get a stack dump of potentially hanging
	// commands.
	if err := syscall.Kill(pid, syscall.SIGQUIT); err != nil {
		fmt.Fprintf(e.opts.stderr, "Kill(%v, %v) failed: %v\n", pid, syscall.SIGQUIT, err)
	}
	fmt.Fprintf(e.opts.stderr, "Waiting for command to exit: %q\n", command.Args)
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
			fmt.Fprintf(e.opts.stderr, "Kill(%v, %v) failed: %v\n", pid, syscall.SIGKILL, err)
		}
	}
}

func (e *executor) decreaseIndent() {
	e.indent--
}

func (e *executor) increaseIndent() {
	e.indent++
}

func (e *executor) printf(stdout io.Writer, format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05.00")
	args = append([]interface{}{timestamp, strings.Repeat(prefix, e.indent)}, args...)
	fmt.Fprintf(stdout, "[%s] %v "+format+"\n", args...)
}
