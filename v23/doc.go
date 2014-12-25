// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
The veyron tool helps manage veyron development.

Usage:
   veyron [flags] <command>

The veyron commands are:
   buildcop     Manage veyron build cop schedule
   contributors List veyron project contributors
   env          Print veyron environment variables
   go           Execute the go tool using the veyron environment
   goext        Veyron extensions of the go tool
   profile      Manage veyron profiles
   project      Manage veyron projects
   run          Run an executable using the veyron environment
   snapshot     Manage snapshots of the veyron project
   test         Manage veyron tests
   update       Update all veyron tools and projects
   version      Print version
   xgo          Execute the go tool using the veyron environment and
                cross-compilation
   help         Display help for commands or topics
Run "veyron help [command]" for command usage.

The veyron flags are:
 -v=false
   Print verbose output.

Veyron Buildcop

Manage veyron build cop schedule. If no subcommand is given, it shows the LDAP
of the current build cop.

Usage:
   veyron buildcop <command>
   veyron buildcop

The veyron buildcop commands are:
   list        List available build cop schedule

Veyron Buildcop List

List available build cop schedule.

Usage:
   veyron buildcop list

Veyron Contributors

Lists veyron project contributors and the number of their commits. Veyron
projects to consider can be specified as an argument. If no projects are
specified, all veyron projects are considered by default.

Usage:
   veyron contributors <projects>

<projects> is a list of projects to consider.

Veyron Env

Print veyron environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format, each on
a separate line ordered by name.  This format makes it easy to set all vars by
running the following bash command (or similar for other shells):
   eval $(veyron env)

If arguments are given, prints only the value of each named variable, each on a
separate line in the same order as the arguments.

Usage:
   veyron env [flags] [name ...]

[name ...] is an optional list of variable names.

The veyron env flags are:
 -platform=
   Target platform.

Veyron Go

Wrapper around the 'go' tool that can be used for compilation of veyron Go
sources. It takes care of veyron-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

In particular, the tool invokes the following command before invoking any go
tool commands that compile veyron Go code:

vdl generate -lang=go all

Usage:
   veyron go [flags] <arg ...>

<arg ...> is a list of arguments for the go tool.

The veyron go flags are:
 -host_go=go
   Go command for the host platform.
 -novdl=false
   Disable automatic generation of vdl files.
 -target_go=go
   Go command for the target platform.

Veyron Goext

Veyron extension of the go tool.

Usage:
   veyron goext <command>

The veyron goext commands are:
   distclean   Restore the veyron Go repositories to their pristine state

Veyron Goext Distclean

Unlike the 'go clean' command, which only removes object files for packages in
the source tree, the 'goext disclean' command removes all object files from
veyron Go workspaces. This functionality is needed to avoid accidental use of
stale object files that correspond to packages that no longer exist in the
source tree.

Usage:
   veyron goext distclean

Veyron Profile

To facilitate development across different platforms, veyron defines
platform-independent profiles that map different platforms to a set of libraries
and tools that can be used for a factor of veyron development.

Usage:
   veyron profile <command>

The veyron profile commands are:
   list        List known veyron profiles
   setup       Set up the given veyron profiles

Veyron Profile List

List known veyron profiles.

Usage:
   veyron profile list

Veyron Profile Setup

Set up the given veyron profiles.

Usage:
   veyron profile setup <profiles>

<profiles> is a list of profiles to set up.

Veyron Project

Manage veyron projects.

Usage:
   veyron project <command>

The veyron project commands are:
   list        List existing veyron projects and branches
   poll        Poll existing veyron projects

Veyron Project List

Inspect the local filesystem and list the existing projects and branches.

Usage:
   veyron project list [flags]

The veyron project list flags are:
 -branches=false
   Show project branches.
 -nopristine=false
   If true, omit pristine projects, i.e. projects with a clean master branch and
   no other branches.

Veyron Project Shell-Prompt

Reports current branches of veyron projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files

Usage:
   veyron project shell-prompt [flags]

The veyron project shell-prompt flags are:
 -check_dirty=true
   If false, don't check for uncommitted changes or untracked files. Setting
   this option to false is dangerous: dirty master branches will not appear in
   the output.
 -show_current_repo_name=false
   Show the name of the current repo.

Veyron Project Poll

Poll veyron projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   veyron project poll <test ...>

<test ...> is a list of tests that determine what projects to poll.

Veyron Run

Run an executable using the veyron environment.

Usage:
   veyron run <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

Veyron Snapshot

The "veyron snapshot" command can be used to manage snapshots of the veyron
project. In particular, it can be used to create new snapshots and to list
existing snapshots.

The command-line flag "-remote" determines whether the command pertains to
"local" snapshots that are only stored locally or "remote" snapshots the are
revisioned in the manifest repository.

Usage:
   veyron snapshot [flags] <command>

The veyron snapshot commands are:
   create      Create a new snapshot of the veyron project
   list        List existing snapshots of veyron projects

The veyron snapshot flags are:
 -remote=false
   Manage remote snapshots.

Veyron Snapshot Create

The "veyron snapshot create <label>" command first checks whether the veyron
tool configuration associates the given label with any tests. If so, the command
checks that all of these tests pass.

Next, the command captures the current state of the veyron project as a manifest
and, depending on the value of the -remote flag, the command either stores the
manifest in the local $VANADIUM_ROOT/.snapshots directory, or in the manifest
repository, pushing the change to the remote repository and thus making it
available globally.

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

NOTE: Unlike the veyron tool commands, the above internal organization is not an
API. It is an implementation and can change without notice.

Usage:
   veyron snapshot create <label>

<label> is the snapshot label.

Veyron Snapshot List

The "snapshot list" command lists existing snapshots of the labels specified as
command-line arguments. If no arguments are provided, the command lists
snapshots for all known labels.

Usage:
   veyron snapshot list <label ...>

<label ...> is a list of snapshot labels.

Veyron Test

Manage veyron tests.

Usage:
   veyron test <command>

The veyron test commands are:
   project     Run tests for a veyron project
   run         Run veyron tests
   list        List veyron tests

Veyron Test Project

Runs tests for a veyron project that is by the remote URL specified as the
command-line argument. Projects hosted on googlesource.com, can be specified
using the basename of the URL (e.g. "veyron.go.core" implies
"https://veyron.googlesource.com/veyron.go.core").

Usage:
   veyron test project <project>

<project> identifies the project for which to run tests.

Veyron Test Run

Run veyron tests.

Usage:
   veyron test run <name ...>

<name ...> is a list names identifying the tests to run.

Veyron Test List

List veyron tests.

Usage:
   veyron test list

Veyron Update

Updates all veyron projects, builds the latest version of veyron tools, and
installs the resulting binaries into $VANADIUM_ROOT/bin. The sequence in which the
individual updates happen guarantees that we end up with a consistent set of
tools and source code.

The set of project and tools to update is describe by a manifest. Veyron
manifests are revisioned and stored in a "manifest" repository, that is
available locally in $VANADIUM_ROOT/.manifest. The manifest uses the following XML
schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="https://veyron.googlesource.com/veyrong.go"
              path="veyron/go/src/veyron.io/veyron"
              protocol="git"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="v23" package="veyron.io/tools/v23"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple manifests.
Import names are interpreted relative to the $VANADIUM_ROOT/.manifest/v1
directory. Import cycles are not allowed and if a project or a tool is specified
multiple times, the last specification takes effect. In particular, the elements
<project name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can
be used to exclude previously included projects and tools.

The tool identifies which manifest to use using the following algorithm. If the
$VANADIUM_ROOT/.local_manifest file exists, then it is used. Otherwise, the
$VANADIUM_ROOT/.manifest/v1/<manifest>.xml file is used, which <manifest> is the
value of the -manifest command-line flag, which defaults to "default".

NOTE: Unlike the veyron tool commands, the above manifest file format is not an
API. It is an implementation and can change without notice.

Usage:
   veyron update [flags]

The veyron update flags are:
 -gc=false
   Garbage collect obsolete repositories.
 -manifest=default
   Name of the project manifest.

Veyron Version

Print version of the veyron tool.

Usage:
   veyron version

Veyron Xgo

Wrapper around the 'go' tool that can be used for cross-compilation of veyron Go
sources. It takes care of veyron-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

In particular, the tool invokes the following command before invoking any go
tool commands that compile veyron Go code:

vdl generate -lang=go all

Usage:
   veyron xgo [flags] <platform> <arg ...>

<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where: - <arch> is the platform
architecture (e.g. 386, amd64 or arm) - <sub> is the platform sub-architecture
(e.g. v6 or v7 for arm) - <os> is the platform operating system (e.g. linux or
darwin) - <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."

The veyron xgo flags are:
 -host_go=go
   Go command for the host platform.
 -novdl=false
   Disable automatic generation of vdl files.
 -target_go=go
   Go command for the target platform.

Veyron Help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

The output is formatted to a target width in runes.  The target width is
determined by checking the environment variable CMDLINE_WIDTH, falling back on
the terminal width from the OS, falling back on 80 chars.  By setting
CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0 the width is unlimited, and
if x == 0 or is unset one of the fallbacks is used.

Usage:
   veyron help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The veyron help flags are:
 -style=text
   The formatting style for help output, either "text" or "godoc".
*/
package main
