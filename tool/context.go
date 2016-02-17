// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tool

import (
	"io"
	"net/url"
	"os"

	"v.io/jiri/gerrit"
	"v.io/jiri/jenkins"
	"v.io/jiri/runutil"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
	"v.io/x/lib/timing"
)

// Context represents an execution context of a tool command
// invocation. Its purpose is to enable sharing of state throughout
// the lifetime of a command invocation.
type Context struct {
	opts ContextOpts
}

// ContextOpts records the context options.
type ContextOpts struct {
	Color    *bool
	Manifest *string
	Env      map[string]string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Verbose  *bool
	Timer    *timing.Timer
}

// newContextOpts is the ContextOpts factory.
func newContextOpts() *ContextOpts {
	return &ContextOpts{
		Color:    &ColorFlag,
		Env:      map[string]string{},
		Manifest: &ManifestFlag,
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Verbose:  &VerboseFlag,
		Timer:    nil,
	}
}

// initOpts initializes all unset options to the given defaults.
func initOpts(defaultOpts, opts *ContextOpts) {
	if opts.Color == nil {
		opts.Color = defaultOpts.Color
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
	if opts.Timer == nil {
		opts.Timer = defaultOpts.Timer
	}
}

// NewContext is the Context factory.
func NewContext(opts ContextOpts) *Context {
	initOpts(newContextOpts(), &opts)
	return &Context{opts: opts}
}

// NewContextFromEnv returns a new context instance based on the given
// cmdline environment.
func NewContextFromEnv(env *cmdline.Env) *Context {
	opts := ContextOpts{}
	initOpts(newContextOpts(), &opts)
	opts.Env = envvar.CopyMap(env.Vars)
	opts.Stdin = env.Stdin
	opts.Stdout = env.Stdout
	opts.Stderr = env.Stderr
	opts.Timer = env.Timer
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
	return false
}

// Env returns the environment of the context.
func (ctx Context) Env() map[string]string {
	return ctx.opts.Env
}

// Gerrit returns the Gerrit instance of the context.
func (ctx Context) Gerrit(host *url.URL) *gerrit.Gerrit {
	return gerrit.New(ctx.NewSeq(), host)
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

// NewSeq returns a new instance of Sequence initialized using the options
// stored in the context.
func (ctx Context) NewSeq() runutil.Sequence {
	return runutil.NewSequence(ctx.opts.Env, ctx.opts.Stdin, ctx.opts.Stdout, ctx.opts.Stderr, *ctx.opts.Color, false, *ctx.opts.Verbose)
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

// Timer returns the timer associated with the context, which may be nil.
func (ctx Context) Timer() *timing.Timer {
	return ctx.opts.Timer
}

// TimerPush calls ctx.Timer().Push(name), only if the Timer is non-nil.
func (ctx Context) TimerPush(name string) {
	if ctx.opts.Timer != nil {
		ctx.opts.Timer.Push(name)
	}
}

// TimerPop calls ctx.Timer().Pop(), only if the Timer is non-nil.
func (ctx Context) TimerPop() {
	if ctx.opts.Timer != nil {
		ctx.opts.Timer.Pop()
	}
}
