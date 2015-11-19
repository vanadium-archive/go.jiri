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

type Opts struct {
	Color   bool
	Dir     string
	DryRun  bool
	Env     map[string]string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Verbose bool
}

type executor struct {
	indent int
	opts   Opts
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
		opts: Opts{
			Color:   color,
			DryRun:  dryRun,
			Env:     env,
			Stdin:   stdin,
			Stdout:  stdout,
			Stderr:  stderr,
			Verbose: verbose,
		},
	}
}

// Opts returns the instance's options.
func (e *executor) Opts() Opts {
	return e.opts
}

// execute executes the binary pointed to by the given path using the given
// arguments and options. If the wait flag is set, the function waits for the
// completion of the binary and the timeout value can optionally specify for
// how long should the function wait before timing out.
func (e *executor) execute(wait bool, timeout time.Duration, opts Opts, path string, args ...string) (*exec.Cmd, error) {
	e.increaseIndent()
	defer e.decreaseIndent()

	// Check if <path> identifies a binary in the PATH environment
	// variable of the opts.Env.
	binary, err := LookPath(path, opts.Env)
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
	command.Dir = opts.Dir
	command.Stdin = opts.Stdin
	command.Stdout = opts.Stdout
	command.Stderr = opts.Stderr
	if len(opts.Env) != 0 {
		vars := envvar.VarsFromOS()
		for key, value := range opts.Env {
			vars.Set(key, value)
		}
		command.Env = vars.ToSlice()
	}
	if opts.Verbose || opts.DryRun {
		args := []string{}
		for _, arg := range command.Args {
			// Quote any arguments that contain '"', ''', '|', or ' '.
			if strings.IndexAny(arg, "\"' |") != -1 {
				args = append(args, strconv.Quote(arg))
			} else {
				args = append(args, arg)
			}
		}
		e.printf(e.opts.Stdout, strings.Replace(strings.Join(args, " "), "%", "%%", -1))
	}
	if opts.DryRun {
		e.printf(e.opts.Stdout, "OK")
		return nil, nil
	}

	if wait {
		if timeout == 0 {
			if err = command.Run(); err != nil {
				if opts.Verbose {
					e.printf(e.opts.Stdout, "FAILED")
				}
			} else {
				if opts.Verbose {
					e.printf(e.opts.Stdout, "OK")
				}
			}
		} else {
			err = e.timedCommand(timeout, opts, command)
		}
	} else {
		err = command.Start()
		if err != nil {
			if opts.Verbose {
				e.printf(e.opts.Stdout, "FAILED")
			}
		} else {
			if opts.Verbose {
				e.printf(e.opts.Stdout, "OK")
			}
		}
	}
	return command, err
}

// timedCommand executes the given command, terminating it forcefully
// if it is still running after the given timeout elapses.
func (e *executor) timedCommand(timeout time.Duration, opts Opts, command *exec.Cmd) error {
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
		if opts.Verbose {
			e.printf(e.opts.Stdout, "FAILED")
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
		if opts.Verbose {
			e.printf(e.opts.Stdout, "TIMED OUT")
		}
		return CommandTimedOutErr
	case err := <-done:
		if err != nil {
			if opts.Verbose {
				e.printf(e.opts.Stdout, "FAILED")
			}
		} else {
			if opts.Verbose {
				e.printf(e.opts.Stdout, "OK")
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
		fmt.Fprintf(e.opts.Stderr, "Kill(%v, %v) failed: %v\n", pid, syscall.SIGQUIT, err)
	}
	fmt.Fprintf(e.opts.Stderr, "Waiting for command to exit: %q\n", command.Args)
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
			fmt.Fprintf(e.opts.Stderr, "Kill(%v, %v) failed: %v\n", pid, syscall.SIGKILL, err)
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
