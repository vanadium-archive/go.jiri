// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The v23 tool helps manage vanadium development.

Usage:
   v23 [flags] <command>

The v23 commands are:
   buildcop     Manage vanadium build cop schedule
   contributors List vanadium project contributors
   env          Print vanadium environment variables
   go           Execute the go tool using the vanadium environment
   goext        Vanadium extensions of the go tool
   profile      Manage vanadium profiles
   project      Manage the vanadium projects
   run          Run an executable using the vanadium environment
   snapshot     Manage snapshots of the vanadium project
   test         Manage vanadium tests
   update       Update all vanadium tools and projects
   version      Print version
   xgo          Execute the go tool using the vanadium environment and
                cross-compilation
   help         Display help for commands or topics
Run "v23 help [command]" for command usage.

The v23 flags are:
 -n=false
   Show what commands will run but do not execute them.
 -nocolor=false
   Do not use color to format output.
 -v=false
   Print verbose output.

V23 Buildcop

Manage vanadium build cop schedule. If no subcommand is given, it shows the LDAP
of the current build cop.

Usage:
   v23 buildcop <command>
   v23 buildcop

The v23 buildcop commands are:
   list        List available build cop schedule

V23 Buildcop List

List available build cop schedule.

Usage:
   v23 buildcop list

V23 Contributors

Lists vanadium project contributors and the number of their commits. Vanadium
projects to consider can be specified as an argument. If no projects are
specified, all vanadium projects are considered by default.

Usage:
   v23 contributors <projects>

<projects> is a list of projects to consider.

V23 Env

Print vanadium environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format, each on
a separate line ordered by name.  This format makes it easy to set all vars by
running the following bash command (or similar for other shells):
   eval $(v23 env)

If arguments are given, prints only the value of each named variable, each on a
separate line in the same order as the arguments.

Usage:
   v23 env [flags] [name ...]

[name ...] is an optional list of variable names.

The v23 env flags are:
 -platform=
   Target platform.

V23 Go

Wrapper around the 'go' tool that can be used for compilation of vanadium Go
sources. It takes care of vanadium-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

In particular, the tool invokes the following command before invoking any go
tool commands that compile vanadium Go code:

vdl generate -lang=go all

Usage:
   v23 go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The v23 go flags are:
 -host_go=go
   Go command for the host platform.
 -target_go=go
   Go command for the target platform.

V23 Goext

Vanadium extension of the go tool.

Usage:
   v23 goext <command>

The v23 goext commands are:
   distclean   Restore the vanadium Go workspaces to their pristine state

V23 Goext Distclean

Unlike the 'go clean' command, which only removes object files for packages in
the source tree, the 'goext disclean' command removes all object files from
vanadium Go workspaces. This functionality is needed to avoid accidental use of
stale object files that correspond to packages that no longer exist in the
source tree.

Usage:
   v23 goext distclean

V23 Profile

To facilitate development across different platforms, vanadium defines
platform-independent profiles that map different platforms to a set of libraries
and tools that can be used for a factor of vanadium development.

Usage:
   v23 profile <command>

The v23 profile commands are:
   list        List known vanadium profiles
   setup       Set up the given vanadium profiles

V23 Profile List

List known vanadium profiles.

Usage:
   v23 profile list

V23 Profile Setup

Set up the given vanadium profiles.

Usage:
   v23 profile setup <profiles>

<profiles> is a list of profiles to set up.

V23 Project

Manage the vanadium projects.

Usage:
   v23 project [flags] <command>

The v23 project commands are:
   list         List existing vanadium projects and branches
   shell-prompt Print a succinct status of projects, suitable for shell prompts
   poll         Poll existing vanadium projects

The v23 project flags are:
 -manifest=default
   Name of the project manifest.

V23 Project List

Inspect the local filesystem and list the existing projects and branches.

Usage:
   v23 project list [flags]

The v23 project list flags are:
 -branches=false
   Show project branches.
 -nopristine=false
   If true, omit pristine projects, i.e. projects with a clean master branch and
   no other branches.

V23 Project Shell-Prompt

Reports current branches of vanadium projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files

Usage:
   v23 project shell-prompt [flags]

The v23 project shell-prompt flags are:
 -check_dirty=true
   If false, don't check for uncommitted changes or untracked files. Setting
   this option to false is dangerous: dirty master branches will not appear in
   the output.
 -show_current_repo_name=false
   Show the name of the current repo.

V23 Project Poll

Poll vanadium projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   v23 project poll <test ...>

<test ...> is a list of tests that determine what projects to poll.

V23 Run

Run an executable using the vanadium environment.

Usage:
   v23 run <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

V23 Snapshot

The "v23 snapshot" command can be used to manage snapshots of the vanadium
project. In particular, it can be used to create new snapshots and to list
existing snapshots.

The command-line flag "-remote" determines whether the command pertains to
"local" snapshots that are only stored locally or "remote" snapshots the are
revisioned in the manifest repository.

Usage:
   v23 snapshot [flags] <command>

The v23 snapshot commands are:
   create      Create a new snapshot of the vanadium project
   list        List existing snapshots of vanadium projects

The v23 snapshot flags are:
 -remote=false
   Manage remote snapshots.

V23 Snapshot Create

The "v23 snapshot create <label>" command first checks whether the vanadium
project configuration associates the given label with any tests. If so, the
command checks that all of these tests pass.

Next, the command captures the current state of the vanadium project as a
manifest and, depending on the value of the -remote flag, the command either
stores the manifest in the local $VANADIUM_ROOT/.snapshots directory, or in the
manifest repository, pushing the change to the remote repository and thus making
it available globally.

Internally, snapshots are organized as follows:

 <snapshot-dir>/
   labels/
     <label1>/
       <label1-snapshot1>
       <label1-snapshot2>
       ...
     <label2>/
       <label2-snapshot1>
       <label2-snapshot2>
       ...
     <label3>/
     ...
   <label1> # a symlink to the latest <label1-snapshot*>
   <label2> # a symlink to the latest <label2-snapshot*>
   ...

NOTE: Unlike the v23 tool commands, the above internal organization is not an
API. It is an implementation and can change without notice.

Usage:
   v23 snapshot create <label>

<label> is the snapshot label.

V23 Snapshot List

The "snapshot list" command lists existing snapshots of the labels specified as
command-line arguments. If no arguments are provided, the command lists
snapshots for all known labels.

Usage:
   v23 snapshot list <label ...>

<label ...> is a list of snapshot labels.

V23 Test

Manage vanadium tests.

Usage:
   v23 test <command>

The v23 test commands are:
   project     Run tests for a vanadium project
   run         Run vanadium tests
   list        List vanadium tests
   generate    Generates supporting code for v23 integration tests.

V23 Test Project

Runs tests for a vanadium project that is by the remote URL specified as the
command-line argument. Projects hosted on googlesource.com, can be specified
using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").

Usage:
   v23 test project <project>

<project> identifies the project for which to run tests.

V23 Test Run

Run vanadium tests.

Usage:
   v23 test run [flags] <name...>

<name...> is a list names identifying the tests to run.

The v23 test run flags are:
 -pkgs=
   comma-separated list of Go package expressions that identify a subset of
   tests to run; only relevant for Go-based tests

V23 Test List

List vanadium tests.

Usage:
   v23 test list

V23 Test Generate

The generate subcommand supports the vanadium integration test framework and
unit tests by generating go files that contain supporting code. v23 test
generate is intended to be invoked via the 'go generate' mechanism and the
resulting files are to be checked in.

Integration tests are functions of the form shown below that are defined in
'external' tests (i.e. those occurring in _test packages, rather than being part
of the package being tested). This ensures that integration tests are isolated
from the packages being tested and can be moved to their own package if need be.
Integration tests have the following form:

    func V23Test<x> (i *v23tests.T)

    'v23 test generate' operates as follows:

In addition, some commonly used functionality in vanadium unit tests is
streamlined. Arguably this should be in a separate command/file but for now they
are lumped together. The additional functionality is as follows:

1. v.io/veyron/lib/modules requires the use of an explicit
   registration mechanism. 'v23 test generate' automatically
   generates these registration functions for any test function matches
   the modules.Main signature.

   For:
   // SubProc does the following...
   // Usage: <a> <b>...
   func SubProc(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error

   It will generate:

   modules.RegisterChild("SubProc",`SubProc does the following...
Usage: <a> <b>...`, SubProc)

2. 'TestMain' is used as the entry point for all vanadium tests, integration
   and otherwise. v23 will generate an appropriate version of this if one is
   not already defined. TestMain is 'special' in that only one definiton can
   occur across both the internal and external test packages. This is a
   consequence of how the go testing system is implemented.

Usage:
   v23 test generate [flags] [packages]

list of go packages

The v23 test generate flags are:
 -output=v23_test.go
   Specifies what file to output the generated code to. Two files are generated,
   <output> and internal_<output>.

V23 Update

Updates all vanadium projects, builds the latest version of vanadium tools, and
installs the resulting binaries into $VANADIUM_ROOT/bin. The sequence in which
the individual updates happen guarantees that we end up with a consistent set of
tools and source code.

The set of project and tools to update is describe by a manifest. Vanadium
manifests are revisioned and stored in a "manifest" repository, that is
available locally in $VANADIUM_ROOT/.manifest. The manifest uses the following
XML schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="https://vanadium.googlesource.com/vanadium.go.core"
              path="release/go/src/v.io/core"
              protocol="git"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="v23" package="v.io/tools/v23"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple manifests.
Import names are interpreted relative to the $VANADIUM_ROOT/.manifest/v2
directory. Import cycles are not allowed and if a project or a tool is specified
multiple times, the last specification takes effect. In particular, the elements
<project name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can
be used to exclude previously included projects and tools.

The tool identifies which manifest to use using the following algorithm. If the
$VANADIUM_ROOT/.local_manifest file exists, then it is used. Otherwise, the
$VANADIUM_ROOT/.manifest/v2/<manifest>.xml file is used, which <manifest> is the
value of the -manifest command-line flag, which defaults to "default".

NOTE: Unlike the v23 tool commands, the above manifest file format is not an
API. It is an implementation and can change without notice.

Usage:
   v23 update [flags]

The v23 update flags are:
 -attempts=1
   Number of attempts before failing.
 -gc=false
   Garbage collect obsolete repositories.
 -manifest=default
   Name of the project manifest.

V23 Version

Print version of the v23 tool.

Usage:
   v23 version

V23 Xgo

Wrapper around the 'go' tool that can be used for cross-compilation of vanadium
Go sources. It takes care of vanadium-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

In particular, the tool invokes the following command before invoking any go
tool commands that compile vanadium Go code:

vdl generate -lang=go all

Usage:
   v23 xgo [flags] <platform> <arg ...>

<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where: - <arch> is the platform
architecture (e.g. 386, amd64 or arm) - <sub> is the platform sub-architecture
(e.g. v6 or v7 for arm) - <os> is the platform operating system (e.g. linux or
darwin) - <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."

The v23 xgo flags are:
 -host_go=go
   Go command for the host platform.
 -target_go=go
   Go command for the target platform.

V23 Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   v23 help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The v23 help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
