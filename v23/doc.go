// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command v23 is a multi-purpose tool for Vanadium development.

Usage:
   v23 [flags] <command>

The v23 commands are:
   api          Work with Vanadium's public API
   buildcop     Manage vanadium build cop schedule
   cl           Manage vanadium changelists
   contributors List vanadium project contributors
   copyright    Manage vanadium copyright
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
   help         Display help for commands or topics

The v23 flags are:
 -color=true
   Use color to format output.
 -n=false
   Show what commands will run but do not execute them.
 -v=false
   Print verbose output.

The global flags are:
 -v23.metadata=<just specify -v23.metadata to activate>
   Displays metadata for the program and exits.

V23 api

Use this command to ensure that no unintended changes are made to Vanadium's
public API.

Usage:
   v23 api [flags] <command>

The v23 api commands are:
   check       Check to see if any changes have been made to the public API.
   fix         Updates the .api files to reflect your changes to the public API.

The v23 api flags are:
 -gotools-bin=
   The path to the gotools binary to use. If empty, gotools will be built if
   necessary.

V23 api check

Check to see if any changes have been made to the public API.

Usage:
   v23 api check [flags] <projects>

<projects> is a list of Vanadium projects to check. If none are specified, all
projects that require a public API check upon presubmit are checked.

The v23 api check flags are:
 -detailed=true
   If true, shows each API change in an expanded form. Otherwise, only a summary
   is shown.

V23 api fix

Updates the .api files to reflect your changes to the public API.

Usage:
   v23 api fix <projects>

<projects> is a list of Vanadium projects to update. If none are specified, all
project APIs are updated.

V23 buildcop

Manage vanadium build cop schedule. If no subcommand is given, it shows the LDAP
of the current build cop.

Usage:
   v23 buildcop <command>
   v23 buildcop

The v23 buildcop commands are:
   list        List available build cop schedule

V23 buildcop list

List available build cop schedule.

Usage:
   v23 buildcop list

V23 cl

Manage vanadium changelists.

Usage:
   v23 cl <command>

The v23 cl commands are:
   cleanup     Clean up branches that have been merged
   mail        Mail a changelist based on the current branch to Gerrit for
               review

V23 cl cleanup

The cleanup command checks that the given branches have been merged into the
master branch. If a branch differs from the master, it reports the difference
and stops. Otherwise, it deletes the branch.

Usage:
   v23 cl cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The v23 cl cleanup flags are:
 -f=false
   Ignore unmerged changes.

V23 cl mail

Squashes all commits of a local branch into a single "changelist" and mails this
changelist to Gerrit as a single commit. First time the command is invoked, it
generates a Change-Id for the changelist, which is appended to the commit
message. Consecutive invocations of the command use the same Change-Id by
default, informing Gerrit that the incomming commit is an update of an existing
changelist.

Usage:
   v23 cl mail [flags]

The v23 cl mail flags are:
 -cc=
   Comma-seperated list of emails or LDAPs to cc.
 -check-api=true
   Check for changes in the public Go API.
 -check-copyright=true
   Check copyright headers.
 -check-depcop=true
   Check that no go-depcop violations exist.
 -check-gofmt=true
   Check that no go fmt violations exist.
 -check-uncommitted=true
   Check that no uncommitted changes exist.
 -d=false
   Send a draft changelist.
 -edit=true
   Open an editor to edit the commit message.
 -presubmit=all
   The type of presubmit tests to run. Valid values: none,all.
 -r=
   Comma-seperated list of emails or LDAPs to request review.

V23 contributors

Lists vanadium project contributors. Vanadium projects to consider can be
specified as an argument. If no projects are specified, all vanadium projects
are considered by default.

Usage:
   v23 contributors [flags] <projects>

<projects> is a list of projects to consider.

The v23 contributors flags are:
 -n=false
   Show number of contributions.

V23 copyright

This command can be used to check if all source code files of Vanadium projects
contain the appropriate copyright header and also if all projects contains the
appropriate licensing files. Optionally, the command can be used to fix the
appropriate copyright headers and licensing files.

Usage:
   v23 copyright [flags] <command>

The v23 copyright commands are:
   check       Check copyright headers and licensing files
   fix         Fix copyright headers and licensing files

The v23 copyright flags are:
 -manifest=
   Name of the project manifest.

V23 copyright check

Check copyright headers and licensing files.

Usage:
   v23 copyright check <projects>

<projects> is a list of projects to check.

V23 copyright fix

Fix copyright headers and licensing files.

Usage:
   v23 copyright fix <projects>

<projects> is a list of projects to fix.

V23 env

Print vanadium environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format, each on
a separate line ordered by name.  This format makes it easy to set all vars by
running the following bash command (or similar for other shells):
   eval $(v23 env)

If arguments are given, prints only the value of each named variable, each on a
separate line in the same order as the arguments.

Usage:
   v23 env [name ...]

[name ...] is an optional list of variable names.

V23 go

Wrapper around the 'go' tool that can be used for compilation of vanadium Go
sources. It takes care of vanadium-specific setup, such as setting up the Go
specific environment variables or making sure that VDL generated files are
regenerated before compilation.

In particular, the tool invokes the following command before invoking any go
tool commands that compile vanadium Go code:

vdl generate -lang=go all

Usage:
   v23 go <arg ...>

<arg ...> is a list of arguments for the go tool.

V23 goext

Vanadium extension of the go tool.

Usage:
   v23 goext <command>

The v23 goext commands are:
   distclean   Restore the vanadium Go workspaces to their pristine state

V23 goext distclean

Unlike the 'go clean' command, which only removes object files for packages in
the source tree, the 'goext disclean' command removes all object files from
vanadium Go workspaces. This functionality is needed to avoid accidental use of
stale object files that correspond to packages that no longer exist in the
source tree.

Usage:
   v23 goext distclean

V23 profile

To facilitate development across different platforms, vanadium defines
platform-independent profiles that map different platforms to a set of libraries
and tools that can be used for a factor of vanadium development.

Usage:
   v23 profile <command>

The v23 profile commands are:
   list        List known vanadium profiles
   setup       Set up the given vanadium profiles

V23 profile list

List known vanadium profiles.

Usage:
   v23 profile list

V23 profile setup

Set up the given vanadium profiles.

Usage:
   v23 profile setup <profiles>

<profiles> is a list of profiles to set up.

V23 project

Manage the vanadium projects.

Usage:
   v23 project <command>

The v23 project commands are:
   clean        Restore vanadium projects to their pristine state
   list         List existing vanadium projects and branches
   shell-prompt Print a succinct status of projects, suitable for shell prompts
   poll         Poll existing vanadium projects

V23 project clean

Restore vanadium projects back to their master branches and get rid of all the
local branches and changes.

Usage:
   v23 project clean [flags] <project ...>

<project ...> is a list of projects to clean up.

The v23 project clean flags are:
 -branches=false
   Delete all non-master branches.

V23 project list

Inspect the local filesystem and list the existing projects and branches.

Usage:
   v23 project list [flags]

The v23 project list flags are:
 -branches=false
   Show project branches.
 -nopristine=false
   If true, omit pristine projects, i.e. projects with a clean master branch and
   no other branches.

V23 project shell-prompt

Reports current branches of vanadium projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files

Usage:
   v23 project shell-prompt [flags]

The v23 project shell-prompt flags are:
 -check-dirty=true
   If false, don't check for uncommitted changes or untracked files. Setting
   this option to false is dangerous: dirty master branches will not appear in
   the output.
 -show-name=false
   Show the name of the current repo.

V23 project poll

Poll vanadium projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   v23 project poll [flags] <test ...>

<test ...> is a list of tests that determine what projects to poll.

The v23 project poll flags are:
 -manifest=
   Name of the project manifest.

V23 run

Run an executable using the vanadium environment.

Usage:
   v23 run <executable> [arg ...]

<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.

V23 snapshot

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

V23 snapshot create

The "v23 snapshot create <label>" command first checks whether the vanadium
project configuration associates the given label with any tests. If so, the
command checks that all of these tests pass.

Next, the command captures the current state of the vanadium project as a
manifest and, depending on the value of the -remote flag, the command either
stores the manifest in the local $V23_ROOT/.snapshots directory, or in the
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
   v23 snapshot create [flags] <label>

<label> is the snapshot label.

The v23 snapshot create flags are:
 -time-format=2006-01-02T15:04:05Z07:00
   Time format for snapshot file name.

V23 snapshot list

The "snapshot list" command lists existing snapshots of the labels specified as
command-line arguments. If no arguments are provided, the command lists
snapshots for all known labels.

Usage:
   v23 snapshot list <label ...>

<label ...> is a list of snapshot labels.

V23 test

Manage vanadium tests.

Usage:
   v23 test <command>

The v23 test commands are:
   generate    Generates supporting code for v23 integration tests.
   project     Run tests for a vanadium project
   run         Run vanadium tests
   list        List vanadium tests

V23 test generate

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

1. v.io/veyron/test/modules requires the use of an explicit
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
 -prefix=v23
   Specifies the prefix to use for generated files. Up to two files may
   generated, the defaults are v23_test.go and v23_internal_test.go, or
   <prefix>_test.go and <prefix>_internal_test.go.
 -progress=false
   Print verbose progress information.

V23 test project

Runs tests for a vanadium project that is by the remote URL specified as the
command-line argument. Projects hosted on googlesource.com, can be specified
using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").

Usage:
   v23 test project <project>

<project> identifies the project for which to run tests.

V23 test run

Run vanadium tests.

Usage:
   v23 test run [flags] <name...>

<name...> is a list names identifying the tests to run.

The v23 test run flags are:
 -blessings-root=dev.v.io
   The blessings root.
 -num-test-workers=<runtime.NumCPU()>
   Set the number of test workers to use; use 1 to serialize all tests.
 -output-dir=
   Directory to output test results into.
 -part=-1
   Specify which part of the test to run.
 -pkgs=
   Comma-separated list of Go package expressions that identify a subset of
   tests to run; only relevant for Go-based tests
 -v23.credentials=
   Directory for vanadium credentials.
 -v23.namespace.root=/ns.dev.v.io:8101
   The namespace root.

V23 test list

List vanadium tests.

Usage:
   v23 test list

V23 update

Updates all vanadium projects, builds the latest version of vanadium tools, and
installs the resulting binaries into $V23_ROOT/devtools/bin. The sequence in
which the individual updates happen guarantees that we end up with a consistent
set of tools and source code.

The set of project and tools to update is describe by a manifest. Vanadium
manifests are revisioned and stored in a "manifest" repository, that is
available locally in $V23_ROOT/.manifest. The manifest uses the following XML
schema:

 <manifest>
   <imports>
     <import name="default"/>
     ...
   </imports>
   <projects>
     <project name="release.go.v23"
              path="release/go/src/v.io/v23"
              protocol="git"
              name="https://vanadium.googlesource.com/release.go.v23"
              revision="HEAD"/>
     ...
   </projects>
   <tools>
     <tool name="v23" package="v.io/x/devtools/v23"/>
     ...
   </tools>
 </manifest>

The <import> element can be used to share settings across multiple manifests.
Import names are interpreted relative to the $V23_ROOT/.manifest/v2 directory.
Import cycles are not allowed and if a project or a tool is specified multiple
times, the last specification takes effect. In particular, the elements <project
name="foo" exclude="true"/> and <tool name="bar" exclude="true"/> can be used to
exclude previously included projects and tools.

The tool identifies which manifest to use using the following algorithm. If the
$V23_ROOT/.local_manifest file exists, then it is used. Otherwise, the
$V23_ROOT/.manifest/v2/<manifest>.xml file is used, which <manifest> is the
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
 -manifest=
   Name of the project manifest.

V23 version

Print version of the v23 tool.

Usage:
   v23 version

V23 help

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Output is formatted to a target width in runes, determined by checking the
CMDLINE_WIDTH environment variable, falling back on the terminal width, falling
back on 80 chars.  By setting CMDLINE_WIDTH=x, if x > 0 the width is x, if x < 0
the width is unlimited, and if x == 0 or is unset one of the fallbacks is used.

Usage:
   v23 help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The v23 help flags are:
 -style=compact
   The formatting style for help output:
      compact - Good for compact cmdline output.
      full    - Good for cmdline output, shows all global flags.
      godoc   - Good for godoc processing.
   Override the default by setting the CMDLINE_STYLE environment variable.
*/
package main
