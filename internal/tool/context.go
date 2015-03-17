package tool

import (
	"io"
	"os"

	"v.io/x/devtools/internal/gerrit"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/hgutil"
	"v.io/x/devtools/internal/jenkins"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/lib/cmdline"
)

// Context represents an execution context of a tool command
// invocation. Its purpose is to enable sharing of state throughout
// the lifetime of a command invocation.
type Context struct {
	opts ContextOpts
	run  *runutil.Run
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

var (
	e = ""
	f = false
	t = true
)

var defaultOpts = ContextOpts{
	Color:    &f,
	DryRun:   &f,
	Env:      map[string]string{},
	Manifest: &e,
	Stdin:    os.Stdin,
	Stdout:   os.Stdout,
	Stderr:   os.Stdout,
	Verbose:  &t,
}

func processOpts(defaultOpts, opts *ContextOpts) {
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
	processOpts(&defaultOpts, &opts)
	run := runutil.New(opts.Env, opts.Stdin, opts.Stdout, opts.Stderr, *opts.Color, *opts.DryRun, *opts.Verbose)
	return &Context{
		opts: opts,
		run:  run,
	}
}

// NewContextFromCommand returns a new context instance based on the
// given command.
func NewContextFromCommand(command *cmdline.Command, opts ContextOpts) *Context {
	processOpts(&defaultOpts, &opts)
	opts.Stdout = command.Stdout()
	opts.Stderr = command.Stderr()
	return NewContext(opts)
}

// NewDefaultContext returns a new default context.
func NewDefaultContext() *Context {
	return NewContext(defaultOpts)
}

// Clone creates a clone of the given context, overriding select
// settings using the given options.
func (ctx Context) Clone(opts ContextOpts) *Context {
	processOpts(&ctx.opts, &opts)
	return NewContext(opts)
}

// Color returns the color setting of the context.
func (ctx Context) Color() bool {
	return ctx.run.Opts().Color
}

// DryRun returns the dry run setting of the context.
func (ctx Context) DryRun() bool {
	return ctx.run.Opts().DryRun
}

// Env returns the environment of the context.
func (ctx Context) Env() map[string]string {
	return ctx.run.Opts().Env
}

// Gerrit returns the Gerrit instance of the context.
func (ctx Context) Gerrit(host, username, password string) *gerrit.Gerrit {
	return gerrit.New(host, username, password)
}

type gitOpt interface {
	gitOpt()
}
type hgOpt interface {
	hgOpt()
}
type RootDirOpt string

func (RootDirOpt) gitOpt() {}
func (RootDirOpt) hgOpt()  {}

// Git returns a new git instance.
//
// This method accepts one optional argument: the repository root to
// use for commands issued by the returned instance. If not specified,
// commands will use the current directory as the repository root.
func (ctx Context) Git(opts ...gitOpt) *gitutil.Git {
	rootDir := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case RootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	return gitutil.New(ctx.run, rootDir)
}

type HgOpts struct {
	Root string
}

// Hg returns a new hg instance.
//
// This method accepts one optional argument: the repository root to
// use for commands issued by the returned instance. If not specified,
// commands will use the current directory as the repository root.
func (ctx Context) Hg(opts ...hgOpt) *hgutil.Hg {
	rootDir := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case RootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	return hgutil.New(ctx.run, rootDir)
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

// Stdin returns the standard input of the context.
func (ctx Context) Stdin() io.Reader {
	return ctx.run.Opts().Stdin
}

// Stdout returns the standard output of the context.
func (ctx Context) Stdout() io.Writer {
	return ctx.run.Opts().Stdout
}

// Stderr returns the standard error output of the context.
func (ctx Context) Stderr() io.Writer {
	return ctx.run.Opts().Stderr
}

// Verbose returns the verbosity setting of the context.
func (ctx Context) Verbose() bool {
	return ctx.run.Opts().Verbose
}
