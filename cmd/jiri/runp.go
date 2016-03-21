// Copyright 2015 The Vanadium Authors, All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"

	"v.io/jiri"
	"v.io/jiri/gitutil"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/profiles/profilesreader"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
	"v.io/x/lib/simplemr"
)

var (
	cmdRunP   *cmdline.Command
	runpFlags runpFlagValues
)

func newRunP() *cmdline.Command {
	return &cmdline.Command{
		Runner: jiri.RunnerFunc(runRunp),
		Name:   "runp",
		Short:  "Run a command in parallel across jiri projects",
		Long: `
Run a command in parallel across one or more jiri projects using the specified
profile target's environment. Commands are run using the shell specified by the
users $SHELL environment variable, or "sh" if that's not set. Thus commands
are run as $SHELL -c "args..."
 `,
		ArgsName: "<command line>",
		ArgsLong: `
	A command line to be run in each project specified by the supplied command
line flags. Any environment variables intended to be evaluated when the
command line is run must be quoted to avoid expansion before being passed to
runp by the shell.
`,
	}
}

type runpFlagValues struct {
	profilescmdline.ReaderFlagValues
	projectKeys      string
	verbose          bool
	interactive      bool
	hasUncommitted   bool
	hasUntracked     bool
	hasGerritMessage bool
	showNamePrefix   bool
	showKeyPrefix    bool
	exitOnError      bool
	collateOutput    bool
	editMessage      bool
	hasBranch        string
}

func registerCommonFlags(flags *flag.FlagSet, values *runpFlagValues) {
	profilescmdline.RegisterReaderFlags(flags, &values.ReaderFlagValues, jiri.ProfilesDBDir)
	flags.BoolVar(&values.verbose, "v", false, "Print verbose logging information")
	flags.StringVar(&values.projectKeys, "projects", "", "A Regular expression specifying project keys to run commands in. By default, runp will use projects that have the same branch checked as the current project unless it is run from outside of a project in which case it will default to using all projects.")
	flags.BoolVar(&values.hasUncommitted, "has-uncommitted", false, "If specified, match projects that have, or have no, uncommitted changes")
	flags.BoolVar(&values.hasUntracked, "has-untracked", false, "If specified, match projects that have, or have no, untracked files")
	flags.BoolVar(&values.hasGerritMessage, "has-gerrit-message", false, "If specified, match branches that have, or have no, gerrit message")
	flags.BoolVar(&values.interactive, "interactive", true, "If set, the command to be run is interactive and should not have its stdout/stderr manipulated. This flag cannot be used with -show-name-prefix, -show-key-prefix or -collate-stdout.")
	flags.BoolVar(&values.showNamePrefix, "show-name-prefix", false, "If set, each line of output from each project will begin with the name of the project followed by a colon. This is intended for use with long running commands where the output needs to be streamed. Stdout and stderr are spliced apart. This flag cannot be used with -interactive, -show-key-prefix or -collate-stdout.")
	flags.BoolVar(&values.showKeyPrefix, "show-key-prefix", false, "If set, each line of output from each project will begin with the key of the project followed by a colon. This is intended for use with long running commands where the output needs to be streamed. Stdout and stderr are spliced apart. This flag cannot be used with -interactive, -show-name-prefix or -collate-stdout")
	flags.BoolVar(&values.collateOutput, "collate-stdout", true, "Collate all stdout output from each parallel invocation and display it as if had been generated sequentially. This flag cannot be used with -show-name-prefix, -show-key-prefix or -interactive.")
	flags.BoolVar(&values.exitOnError, "exit-on-error", false, "If set, all commands will killed as soon as one reports an error, otherwise, each will run to completion.")
	flags.StringVar(&values.hasBranch, "has-branch", "", "A regular expression specifying branch names to use in matching projects. A project will match if the specified branch exists, even if it is not checked out.")
}

func init() {
	// Avoid an intialization loop between cmdline.Command.Runner which
	// refers to cmdRunP and runRunp referring back to cmdRunP.ParsedFlags.
	cmdRunP = newRunP()
	cmdRoot.Children = append(cmdRoot.Children, cmdRunP)
	registerCommonFlags(&cmdRunP.Flags, &runpFlags)
}

type mapInput struct {
	*project.ProjectState
	key          project.ProjectKey
	jirix        *jiri.X
	index, total int
	result       error
}

func newmapInput(jirix *jiri.X, state *project.ProjectState, key project.ProjectKey, index, total int) *mapInput {
	return &mapInput{
		ProjectState: state,
		key:          key,
		jirix:        jirix.Clone(tool.ContextOpts{}),
		index:        index,
		total:        total,
	}
}

func stateNames(states map[project.ProjectKey]*mapInput) []string {
	n := []string{}
	for _, state := range states {
		n = append(n, state.Project.Name)
	}
	sort.Strings(n)
	return n
}

func stateKeys(states map[project.ProjectKey]*mapInput) []string {
	n := []string{}
	for key := range states {
		n = append(n, string(key))
	}
	sort.Strings(n)
	return n
}

type runner struct {
	args                 []string
	reader               *profilesreader.Reader
	serializedWriterLock sync.Mutex
	collatedOutputLock   sync.Mutex
}

func (r *runner) serializedWriter(w io.Writer) io.Writer {
	return &sharedLockWriter{&r.serializedWriterLock, w}
}

type sharedLockWriter struct {
	mu *sync.Mutex
	f  io.Writer
}

func (lw *sharedLockWriter) Write(d []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.f.Write(d)
}

func copyWithPrefix(prefix string, w io.Writer, r io.Reader) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if line != "" {
				fmt.Fprintf(w, "%v: %v\n", prefix, line)
			}
			break
		}
		fmt.Fprintf(w, "%v: %v", prefix, line)
	}
}

type mapOutput struct {
	mi             *mapInput
	outputFilename string
	key            string
	err            error
}

func (r *runner) Map(mr *simplemr.MR, key string, val interface{}) error {
	mi := val.(*mapInput)
	output := &mapOutput{
		key: key,
		mi:  mi}
	jirix := mi.jirix
	path := os.Getenv("SHELL")
	if path == "" {
		path = "sh"
	}
	var wg sync.WaitGroup
	cmd := exec.Command(path, "-c", strings.Join(r.args, " "))
	cmd.Env = envvar.MapToSlice(jirix.Env())
	cmd.Dir = mi.ProjectState.Project.Path
	cmd.Stdin = mi.jirix.Stdin()
	var stdoutCloser, stderrCloser io.Closer
	if runpFlags.interactive {
		cmd.Stdout = jirix.Stdout()
		cmd.Stderr = jirix.Stderr()
	} else {
		var stdout io.Writer
		stderr := r.serializedWriter(jirix.Stderr())
		var cleanup func()
		if runpFlags.collateOutput {
			// Write standard output to a file, stderr
			// is not collated.
			f, err := ioutil.TempFile("", mi.ProjectState.Project.Name+"-")
			if err != nil {
				return err
			}
			stdout = f
			output.outputFilename = f.Name()
			cleanup = func() {
				os.Remove(output.outputFilename)
			}
			// The child process will have exited by the
			// time this method returns so it's safe to close the file
			// here.
			defer f.Close()
		} else {
			stdout = r.serializedWriter(jirix.Stdout())
			cleanup = func() {}
		}
		if !runpFlags.showNamePrefix && !runpFlags.showKeyPrefix {
			// write directly to stdout, stderr if there's no prefix
			cmd.Stdout = stdout
			cmd.Stderr = stderr
		} else {
			stdoutReader, stdoutWriter, err := os.Pipe()
			if err != nil {
				cleanup()
				return err
			}
			stderrReader, stderrWriter, err := os.Pipe()
			if err != nil {
				cleanup()
				stdoutReader.Close()
				stdoutWriter.Close()
				return err
			}
			cmd.Stdout = stdoutWriter
			cmd.Stderr = stderrWriter
			// Record the write end of the pipe so that it can be closed
			// after the child has exited, this ensures that all goroutines
			// will finish.
			stdoutCloser = stdoutWriter
			stderrCloser = stderrWriter
			prefix := key
			if runpFlags.showNamePrefix {
				prefix = mi.ProjectState.Project.Name
			}
			wg.Add(2)
			go func() { copyWithPrefix(prefix, stdout, stdoutReader); wg.Done() }()
			go func() { copyWithPrefix(prefix, stderr, stderrReader); wg.Done() }()

		}
	}
	if err := cmd.Start(); err != nil {
		mi.result = err
	}
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case output.err = <-done:
		if output.err != nil && runpFlags.exitOnError {
			mr.Cancel()
		}
	case <-mr.CancelCh():
		output.err = cmd.Process.Kill()
	}
	for _, closer := range []io.Closer{stdoutCloser, stderrCloser} {
		if closer != nil {
			closer.Close()
		}
	}
	wg.Wait()
	mr.MapOut(key, output)
	return nil
}

func (r *runner) Reduce(mr *simplemr.MR, key string, values []interface{}) error {
	for _, v := range values {
		mo := v.(*mapOutput)
		jirix := mo.mi.jirix
		if mo.err != nil {
			fmt.Fprintf(jirix.Stdout(), "FAILED: %v: %s %v\n", mo.key, strings.Join(r.args, " "), mo.err)
			return mo.err
		} else {
			if runpFlags.collateOutput {
				r.collatedOutputLock.Lock()
				defer r.collatedOutputLock.Unlock()
				defer os.Remove(mo.outputFilename)
				if fi, err := os.Open(mo.outputFilename); err == nil {
					io.Copy(jirix.Stdout(), fi)
					fi.Close()
				} else {
					return err
				}
			}
		}
	}
	return nil
}

func runp(jirix *jiri.X, cmd *cmdline.Command, args []string) error {
	hasUntrackedSet := profilescmdline.IsFlagSet(cmd.ParsedFlags, "has-untracked")
	hasUncommitedSet := profilescmdline.IsFlagSet(cmd.ParsedFlags, "has-uncommitted")
	hasGerritSet := profilescmdline.IsFlagSet(cmd.ParsedFlags, "has-gerrit-message")

	if runpFlags.interactive {
		runpFlags.collateOutput = false
	}

	var keysRE, branchRE *regexp.Regexp
	var err error

	if profilescmdline.IsFlagSet(cmd.ParsedFlags, "projects") {
		keysRE, err = regexp.Compile(runpFlags.projectKeys)
		if err != nil {
			return fmt.Errorf("failed to compile projects regexp: %q: %v", runpFlags.projectKeys, err)
		}
	}

	if profilescmdline.IsFlagSet(cmd.ParsedFlags, "has-branch") {
		branchRE, err = regexp.Compile(runpFlags.hasBranch)
		if err != nil {
			return fmt.Errorf("failed to compile has-branch regexp: %q: %v", runpFlags.hasBranch, err)
		}
	}

	for _, f := range []string{"show-key-prefix", "show-name-prefix"} {
		if profilescmdline.IsFlagSet(cmd.ParsedFlags, f) {
			runpFlags.interactive = false
			runpFlags.collateOutput = true
			break
		}
	}

	git := gitutil.New(jirix.NewSeq())
	homeBranch, err := git.CurrentBranchName()
	if err != nil {
		// jiri was run from outside of a project, so let's assume we'll
		// all projects if none have been specified.
		if keysRE == nil {
			keysRE = regexp.MustCompile(".*")
		}
	}

	dirty := false
	if hasUntrackedSet || hasUncommitedSet {
		dirty = true
	}
	states, err := project.GetProjectStates(jirix, dirty)
	if err != nil {
		return err
	}
	mapInputs := map[project.ProjectKey]*mapInput{}
	var keys project.ProjectKeys
	for key, state := range states {
		if keysRE != nil {
			if !keysRE.MatchString(string(key)) {
				continue
			}
		} else {
			if state.CurrentBranch != homeBranch {
				continue
			}
		}
		if branchRE != nil {
			found := false
			for _, br := range state.Branches {
				if branchRE.MatchString(br.Name) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if hasUntrackedSet && (state.HasUntracked != runpFlags.hasUntracked) {
			continue
		}
		if hasUncommitedSet && (state.HasUncommitted != runpFlags.hasUncommitted) {
			continue
		}
		if hasGerritSet {
			hasMsg := false
			for _, br := range state.Branches {
				if (state.CurrentBranch == br.Name) && br.HasGerritMessage {
					hasMsg = true
					break
				}
			}
			if hasMsg != runpFlags.hasGerritMessage {
				continue
			}
		}
		mapInputs[key] = &mapInput{
			ProjectState: state,
			jirix:        jirix,
			key:          key,
		}
		keys = append(keys, key)
	}

	total := len(mapInputs)
	index := 1
	for _, mi := range mapInputs {
		mi.index = index
		mi.total = total
		index++
	}

	if runpFlags.verbose {
		fmt.Fprintf(jirix.Stdout(), "Project Names: %s\n", strings.Join(stateNames(mapInputs), " "))
		fmt.Fprintf(jirix.Stdout(), "Project Keys: %s\n", strings.Join(stateKeys(mapInputs), " "))
	}

	reader, err := profilesreader.NewReader(jirix, runpFlags.ProfilesMode, runpFlags.DBFilename)
	runner := &runner{
		reader: reader,
		args:   args,
	}
	mr := simplemr.MR{}
	if runpFlags.interactive {
		// Run one mapper at a time.
		mr.NumMappers = 1
		sort.Sort(keys)
	}
	in, out := make(chan *simplemr.Record, len(mapInputs)), make(chan *simplemr.Record, len(mapInputs))
	sigch := make(chan os.Signal)
	signal.Notify(sigch, os.Interrupt)
	go func() { <-sigch; mr.Cancel() }()
	go mr.Run(in, out, runner, runner)
	for _, key := range keys {
		in <- &simplemr.Record{string(key), []interface{}{mapInputs[key]}}
	}
	close(in)
	<-out
	return mr.Error()
}

func runRunp(jirix *jiri.X, args []string) error {
	return runp(jirix, cmdRunP, args)
}
