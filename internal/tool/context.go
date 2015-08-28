// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tool

import (
	"io"
	"os"

	"v.io/x/devtools/internal/gerrit"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/jenkins"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/lib/cmdline"
)

// Context represents an execution context of a tool command
// invocation. Its purpose is to enable sharing of state throughout
// the lifetime of a command invocation.
type Context struct {
	opts  ContextOpts
	run   *runutil.Run
	start *runutil.Start
}

// ContextOpts records the context options.
type ContextOpts struct {
	Color    *bool
	DryRun   *bool
	Env      map[string]string
	Manifest *string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Verbose  *bool
}

// newContextOpts is the ContextOpts factory.
func newContextOpts() *ContextOpts {
	return &ContextOpts{
		Color:    &ColorFlag,
		DryRun:   &DryRunFlag,
		Env:      map[string]string{},
		Manifest: &ManifestFlag,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Verbose:  &VerboseFlag,
	}
}

// initOpts initializes all unset options to the given defaults.
func initOpts(defaultOpts, opts *ContextOpts) {
	if opts.Color == nil {
		opts.Color = defaultOpts.Color
	}
	if opts.DryRun == nil {
		opts.DryRun = defaultOpts.DryRun
	}
	if opts.Env == nil {
		opts.Env = defaultOpts.Env
	}
	if opts.Manifest == nil {
		opts.Manifest = defaultOpts.Manifest
	}
	if opts.Stdin == nil {
		opts.Stdin = defaultOpts.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = defaultOpts.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = defaultOpts.Stderr
	}
	if opts.Verbose == nil {
		opts.Verbose = defaultOpts.Verbose
	}
}

// NewContext is the Context factory.
func NewContext(opts ContextOpts) *Context {
	initOpts(newContextOpts(), &opts)
	run := runutil.NewRun(opts.Env, opts.Stdin, opts.Stdout, opts.Stderr, *opts.Color, *opts.DryRun, *opts.Verbose)
	start := runutil.NewStart(opts.Env, opts.Stdin, opts.Stdout, opts.Stderr, *opts.Color, *opts.DryRun, *opts.Verbose)
	return &Context{
		opts:  opts,
		run:   run,
		start: start,
	}
}

// NewContextFromEnv returns a new context instance based on the given
// cmdline environment.
func NewContextFromEnv(env *cmdline.Env) *Context {
	opts := ContextOpts{}
	initOpts(newContextOpts(), &opts)
	opts.Stdin = env.Stdin
	opts.Stdout = env.Stdout
	opts.Stderr = env.Stderr
	return NewContext(opts)
}

// NewDefaultContext returns a new default context.
func NewDefaultContext() *Context {
	return NewContext(ContextOpts{})
}

// Clone creates a clone of the given context, overriding select
// settings using the given options.
func (ctx Context) Clone(opts ContextOpts) *Context {
	initOpts(&ctx.opts, &opts)
	return NewContext(opts)
}

// Color returns the color setting of the context.
func (ctx Context) Color() bool {
	return *ctx.opts.Color
}

// DryRun returns the dry run setting of the context.
func (ctx Context) DryRun() bool {
	return *ctx.opts.DryRun
}

// Env returns the environment of the context.
func (ctx Context) Env() map[string]string {
	return ctx.opts.Env
}

// Gerrit returns the Gerrit instance of the context.
func (ctx Context) Gerrit(host string) *gerrit.Gerrit {
	return gerrit.New(ctx.run, host)
}

type gitOpt interface {
	gitOpt()
}
type AuthorDateOpt string
type CommitterDateOpt string
type RootDirOpt string

func (AuthorDateOpt) gitOpt()    {}
func (CommitterDateOpt) gitOpt() {}
func (RootDirOpt) gitOpt()       {}

// Git returns a new git instance.
//
// This method accepts one optional argument: the repository root to
// use for commands issued by the returned instance. If not specified,
// commands will use the current directory as the repository root.
func (ctx Context) Git(opts ...gitOpt) *gitutil.Git {
	rootDir := ""
	gitCtx := &ctx
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case AuthorDateOpt:
			opts := ContextOpts{}
			opts.Env = ctx.Env()
			opts.Env["GIT_AUTHOR_DATE"] = string(typedOpt)
			gitCtx = ctx.Clone(opts)
		case CommitterDateOpt:
			opts := ContextOpts{}
			opts.Env = ctx.Env()
			opts.Env["GIT_COMMITTER_DATE"] = string(typedOpt)
			gitCtx = ctx.Clone(opts)
		case RootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	return gitutil.New(gitCtx.run, rootDir)
}

// Jenkins returns a new Jenkins instance that can be used to
// communicate with a Jenkins server running at the given host.
func (ctx Context) Jenkins(host string) (*jenkins.Jenkins, error) {
	return jenkins.New(host)
}

// Manifest returns the manifest of the context.
func (ctx Context) Manifest() string {
	return *ctx.opts.Manifest
}

// Run returns the run instance of the context.
func (ctx Context) Run() *runutil.Run {
	return ctx.run
}

// Start returns the start instance of the context.
func (ctx Context) Start() *runutil.Start {
	return ctx.start
}

// Stdin returns the standard input of the context.
func (ctx Context) Stdin() io.Reader {
	return ctx.opts.Stdin
}

// Stdout returns the standard output of the context.
func (ctx Context) Stdout() io.Writer {
	return ctx.opts.Stdout
}

// Stderr returns the standard error output of the context.
func (ctx Context) Stderr() io.Writer {
	return ctx.opts.Stderr
}

// Verbose returns the verbosity setting of the context.
func (ctx Context) Verbose() bool {
	return *ctx.opts.Verbose
}
