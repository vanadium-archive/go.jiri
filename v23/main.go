// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const warning = `
############################################################################
# WARNING: Forcefully terminating the operation of the v23 tool can leave  #
# V23_ROOT in an inconsistent state. To terminate v23, press CTRL-C again. #
############################################################################
`

func signalHandler(c chan os.Signal, p *os.Process) {
	warned := false
	for sig := range c {
		switch sig {
		case os.Interrupt:
			if !warned {
				fmt.Fprintf(os.Stdout, "%v", warning)
				warned = true
				continue
			}
		}
		if err := p.Signal(sig); err != nil {
			fmt.Fprintf(os.Stderr, "Signal(%v) failed: %v\n", sig, err)
		}
	}
}

func main() {
	// If the operation of the v23 tool is forcefully terminated, it can
	// leave the V23_ROOT directory in an inconsistent state. To lower
	// the chance of users unintenionally corrupting their V23_ROOT, the
	// v23 tool prints an informative warning message upon receiving the
	// first SIGINT signal instead of terminating itself.
	//
	// Since the v23 tool spawns subprocesses such as git, simply
	// catching the SIGINT signal in the v23 process is not sufficient
	// because pressing CTRL-C in a shell delivers the SIGINT signal to
	// all processes in the foreground process group.
	//
	// To assume control over which signals are delivered to the v23
	// subprocesses, when the v23 tool starts, it spawns a copy of
	// itself that runs under a different process group. The parent
	// process handles incoming signals, while the child process
	// executes the logic identified by the command-line arguments.
	if os.Getenv("V23_CHILD") == "" {
		os.Setenv("V23_CHILD", "1")
		attr := &os.ProcAttr{
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
			Sys:   &syscall.SysProcAttr{Setpgid: true},
		}
		path, err := exec.LookPath(os.Args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "LookPath(%v) failed: %v\n", os.Args[0], err)
			os.Exit(1)
		}
		p, err := os.StartProcess(path, os.Args, attr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "StartProcess() failed: %v\n", err)
			os.Exit(1)
		}
		// Create a channel for signal delivery. Its size determines the
		// maximum number of signals that can be delivered concurrently
		// before signals are dropped.
		const maxConcurrentSignals = 10
		c := make(chan os.Signal, maxConcurrentSignals)
		signal.Notify(c)
		go signalHandler(c, p)
		state, err := p.Wait()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Wait() failed: %v\n", err)
			os.Exit(1)
		}
		status, ok := state.Sys().(syscall.WaitStatus)
		if !ok {
			fmt.Fprintf(os.Stderr, "failed to retrieve the exit status\n")
			os.Exit(1)
		}
		signal.Stop(c)
		close(c)
		os.Exit(status.ExitStatus())
	}
	os.Exit(root().Main())
}
