// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Command jiri is a multi-purpose tool for multi-repo development.

Usage:
   jiri [flags] <command>

The jiri commands are:
   cl           Manage project changelists
   contributors List project contributors
   import       Adds imports to .jiri_manifest file
   profile      Display information about installed profiles
   project      Manage the jiri projects
   rebuild      Rebuild all jiri tools
   snapshot     Manage project snapshots
   update       Update all jiri tools and projects
   which        Show path to the jiri tool
   runp         Run a command in parallel across jiri projects
   help         Display help for commands or topics

The jiri additional help topics are:
   filesystem  Description of jiri file system layout
   manifest    Description of manifest files

The jiri flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri cl - Manage project changelists

Manage project changelists.

Usage:
   jiri cl [flags] <command>

The jiri cl commands are:
   cleanup     Clean up changelists that have been merged
   mail        Mail a changelist for review
   new         Create a new local branch for a changelist
   sync        Bring a changelist up to date

The jiri cl flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri cl cleanup - Clean up changelists that have been merged

Command "cleanup" checks that the given branches have been merged into the
corresponding remote branch. If a branch differs from the corresponding remote
branch, the command reports the difference and stops. Otherwise, it deletes the
given branches.

Usage:
   jiri cl cleanup [flags] <branches>

<branches> is a list of branches to cleanup.

The jiri cl cleanup flags are:
 -f=false
   Ignore unmerged changes.
 -remote-branch=master
   Name of the remote branch the CL pertains to, without the leading "origin/".

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri cl mail - Mail a changelist for review

Command "mail" squashes all commits of a local branch into a single "changelist"
and mails this changelist to Gerrit as a single commit. First time the command
is invoked, it generates a Change-Id for the changelist, which is appended to
the commit message. Consecutive invocations of the command use the same
Change-Id by default, informing Gerrit that the incomming commit is an update of
an existing changelist.

Usage:
   jiri cl mail [flags]

The jiri cl mail flags are:
 -autosubmit=false
   Automatically submit the changelist when feasible.
 -cc=
   Comma-seperated list of emails or LDAPs to cc.
 -check-uncommitted=true
   Check that no uncommitted changes exist.
 -d=false
   Send a draft changelist.
 -edit=true
   Open an editor to edit the CL description.
 -host=
   Gerrit host to use.  Defaults to gerrit host specified in manifest.
 -m=
   CL description.
 -presubmit=all
   The type of presubmit tests to run. Valid values: none,all.
 -r=
   Comma-seperated list of emails or LDAPs to request review.
 -remote-branch=master
   Name of the remote branch the CL pertains to, without the leading "origin/".
 -set-topic=true
   Set Gerrit CL topic.
 -topic=
   CL topic, defaults to <username>-<branchname>.
 -verify=true
   Run pre-push git hooks.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri cl new - Create a new local branch for a changelist

Command "new" creates a new local branch for a changelist. In particular, it
forks a new branch with the given name from the current branch and records the
relationship between the current branch and the new branch in the .jiri metadata
directory. The information recorded in the .jiri metadata directory tracks
dependencies between CLs and is used by the "jiri cl sync" and "jiri cl mail"
commands.

Usage:
   jiri cl new [flags] <name>

<name> is the changelist name.

The jiri cl new flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri cl sync - Bring a changelist up to date

Command "sync" brings the CL identified by the current branch up to date with
the branch tracking the remote branch this CL pertains to. To do that, the
command uses the information recorded in the .jiri metadata directory to
identify the sequence of dependent CLs leading to the current branch. The
command then iterates over this sequence bringing each of the CLs up to date
with its ancestor. The end result of this process is that all CLs in the
sequence are up to date with the branch that tracks the remote branch this CL
pertains to.

NOTE: It is possible that the command cannot automatically merge changes in an
ancestor into its dependent. When that occurs, the command is aborted and prints
instructions that need to be followed before the command can be retried.

Usage:
   jiri cl sync [flags]

The jiri cl sync flags are:
 -remote-branch=master
   Name of the remote branch the CL pertains to, without the leading "origin/".

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri contributors - List project contributors

Lists project contributors. Projects to consider can be specified as an
argument. If no projects are specified, all projects in the current manifest are
considered by default.

Usage:
   jiri contributors [flags] <projects>

<projects> is a list of projects to consider.

The jiri contributors flags are:
 -aliases=
   Path to the aliases file.
 -n=false
   Show number of contributions.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri import

Command "import" adds imports to the $JIRI_ROOT/.jiri_manifest file, which
specifies manifest information for the jiri tool.  The file is created if it
doesn't already exist, otherwise additional imports are added to the existing
file.

An <import> element is added to the manifest representing a remote manifest
import.  The manifest file path is relative to the root directory of the remote
import repository.

Example:
  $ jiri import myfile https://foo.com/bar.git

Run "jiri help manifest" for details on manifests.

Usage:
   jiri import [flags] <manifest> <remote>

<manifest> specifies the manifest file to use.

<remote> specifies the remote manifest repository.

The jiri import flags are:
 -name=
   The name of the remote manifest project, used to disambiguate manifest
   projects with the same remote.  Typically empty.
 -out=
   The output file.  Uses $JIRI_ROOT/.jiri_manifest if unspecified.  Uses stdout
   if set to "-".
 -overwrite=false
   Write a new .jiri_manifest file with the given specification.  If it already
   exists, the existing content will be ignored and the file will be
   overwritten.
 -protocol=git
   The version control protocol used by the remote manifest project.
 -remote-branch=master
   The branch of the remote manifest project to track, without the leading
   "origin/".
 -root=
   Root to store the manifest project locally.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri profile - Display information about installed profiles

Display information about installed profiles and their configuration.

Usage:
   jiri profile [flags] <command>

The jiri profile commands are:
   list        List available or installed profiles
   env         Display profile environment variables
   install     Install the given profiles
   uninstall   Uninstall the given profiles
   update      Install the latest default version of the given profiles
   cleanup     Cleanup the locally installed profiles
   available   List the available profiles

The jiri profile flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri profile list - List available or installed profiles

List available or installed profiles.

Usage:
   jiri profile list [flags] [<profiles>]

<profiles> is a list of profiles to list, defaulting to all profiles if none are
specifically requested. List can also be used to test for the presence of a
specific target for the requested profiles. If the target is not installed, it
will exit with an error.

The jiri profile list flags are:
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -info=
   The following fields for use with -info are available:
   	SchemaVersion - the version of the profiles implementation.
   	DBPath - the path for the profiles database.
   	Target.InstallationDir - the installation directory of the requested profile.
   	Target.CommandLineEnv - the environment variables specified via the command line when installing this profile target.
   	Target.Env - the environment variables computed by the profile installation process for this target.
   	Target.Command - a command that can be used to create this profile.
   	Note: if no --target is specified then the requested field will be displayed for all targets.

   	Profile.Root - the root directory of the requested profile.
   	Profile.Name - the qualified name of the profile.
   	Profile.Installer - the name of the profile installer.
   	Profile.DBPath - the path to the database file for this profile.
   	Note: if no profiles are specified then the requested field will be displayed for all profiles.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile env - Display profile environment variables

List profile specific and target specific environment variables. If the
requested environment variable name ends in = then only the value will be
printed, otherwise both name and value are printed, i.e. GOPATH="foo" vs just
"foo".

If no environment variable names are requested then all will be printed in
<name>=<val> format.

Usage:
   jiri profile env [flags] [<environment variable names>]

[<environment variable names>] is an optional list of environment variables to
display

The jiri profile env flags are:
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile install - Install the given profiles

Install the given profiles.

Usage:
   jiri profile install [flags] <profiles>

<profiles> is a list of profiles to install.

The jiri profile install flags are:
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -force=false
   force install the profile even if it is already installed
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -profiles-dir=.jiri_root/profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri profile uninstall - Uninstall the given profiles

Uninstall the given profiles.

Usage:
   jiri profile uninstall [flags] <profiles>

<profiles> is a list of profiles to uninstall.

The jiri profile uninstall flags are:
 -all-targets=false
   apply to all targets for the specified profile(s)
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -profiles-dir=.jiri_root/profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile update - Install the latest default version of the given profiles

Install the latest default version of the given profiles.

Usage:
   jiri profile update [flags] <profiles>

<profiles> is a list of profiles to update, if omitted all profiles are updated.

The jiri profile update flags are:
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -profiles-dir=.jiri_root/profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile cleanup - Cleanup the locally installed profiles

Cleanup the locally installed profiles. This is generally required when
recovering from earlier bugs or when preparing for a subsequent change to the
profiles implementation.

Usage:
   jiri profile cleanup [flags] <profiles>

<profiles> is a list of profiles to cleanup, if omitted all profiles are
cleaned.

The jiri profile cleanup flags are:
 -gc=false
   uninstall profile targets that are older than the current default
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -profiles-dir=.jiri_root/profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -rewrite-profiles-db=false
   rewrite the profiles database to use the latest schema version
 -rm-all=false
   remove profiles database and all profile generated output files.
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile available - List the available profiles

List the available profiles.

Usage:
   jiri profile available [flags]

The jiri profile available flags are:
 -describe=false
   print the profile description
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri project - Manage the jiri projects

Manage the jiri projects.

Usage:
   jiri project [flags] <command>

The jiri project commands are:
   clean        Restore jiri projects to their pristine state
   info         Provided structured input for existing jiri projects and
                branches
   list         List existing jiri projects and branches
   shell-prompt Print a succinct status of projects suitable for shell prompts
   poll         Poll existing jiri projects

The jiri project flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri project clean - Restore jiri projects to their pristine state

Restore jiri projects back to their master branches and get rid of all the local
branches and changes.

Usage:
   jiri project clean [flags] <project ...>

<project ...> is a list of projects to clean up.

The jiri project clean flags are:
 -branches=false
   Delete all non-master branches.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri project info - Provided structured input for existing jiri projects and branches

Inspect the local filesystem and provide structured info on the existing
projects and branches. Projects are specified using regular expressions that are
matched against project keys. If no command line arguments are provided the
project that the contains the current directory is used, or if run from outside
of a given project, all projects will be used. The information to be displayed
is specified using a go template, supplied via the -f flag, that is executed
against the v.io/jiri/project.ProjectState structure. This structure currently
has the following fields:
project.ProjectState{Branches:[]project.BranchState(nil), CurrentBranch:"",
HasUncommitted:false, HasUntracked:false, Project:project.Project{Name:"",
Path:"", Protocol:"", Remote:"", RemoteBranch:"", Revision:"", GerritHost:"",
GitHooks:"", RunHook:"", XMLName:struct {}{}}}

Usage:
   jiri project info [flags] <project-keys>...

<project-keys>... a list of project keys, as regexps, to apply the specified
format to

The jiri project info flags are:
 -f={{.Project.Name}}
   The go template for the fields to display.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri project list - List existing jiri projects and branches

Inspect the local filesystem and list the existing projects and branches.

Usage:
   jiri project list [flags]

The jiri project list flags are:
 -branches=false
   Show project branches.
 -nopristine=false
   If true, omit pristine projects, i.e. projects with a clean master branch and
   no other branches.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri project shell-prompt - Print a succinct status of projects suitable for shell prompts

Reports current branches of jiri projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files

Usage:
   jiri project shell-prompt [flags]

The jiri project shell-prompt flags are:
 -check-dirty=true
   If false, don't check for uncommitted changes or untracked files. Setting
   this option to false is dangerous: dirty master branches will not appear in
   the output.
 -show-name=false
   Show the name of the current repo.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri project poll - Poll existing jiri projects

Poll jiri projects that can affect the outcome of the given tests and report
whether any new changes in these projects exist. If no tests are specified, all
projects are polled by default.

Usage:
   jiri project poll [flags] <test ...>

<test ...> is a list of tests that determine what projects to poll.

The jiri project poll flags are:
 -manifest=
   Name of the project manifest.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri rebuild - Rebuild all jiri tools

Rebuilds all jiri tools and installs the resulting binaries into
$JIRI_ROOT/.jiri_root/bin. This is similar to "jiri update", but does not update
any projects before building the tools. The set of tools to rebuild is described
in the manifest.

Run "jiri help manifest" for details on manifests.

Usage:
   jiri rebuild [flags]

The jiri rebuild flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri snapshot - Manage project snapshots

The "jiri snapshot" command can be used to manage project snapshots. In
particular, it can be used to create new snapshots and to list existing
snapshots.

Usage:
   jiri snapshot [flags] <command>

The jiri snapshot commands are:
   checkout    Checkout a project snapshot
   create      Create a new project snapshot
   list        List existing project snapshots

The jiri snapshot flags are:
 -dir=
   Directory where snapshot are stored.  Defaults to $JIRI_ROOT/.snapshot.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri snapshot checkout - Checkout a project snapshot

The "jiri snapshot checkout <snapshot>" command restores local project state to
the state in the given snapshot manifest.

Usage:
   jiri snapshot checkout [flags] <snapshot>

<snapshot> is the snapshot manifest file.

The jiri snapshot checkout flags are:
 -gc=false
   Garbage collect obsolete repositories.

 -color=true
   Use color to format output.
 -dir=
   Directory where snapshot are stored.  Defaults to $JIRI_ROOT/.snapshot.
 -v=false
   Print verbose output.

Jiri snapshot create - Create a new project snapshot

The "jiri snapshot create <label>" command captures the current project state in
a manifest.  If the -push-remote flag is provided, the snapshot is committed and
pushed upstream.

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

NOTE: Unlike the jiri tool commands, the above internal organization is not an
API. It is an implementation and can change without notice.

Usage:
   jiri snapshot create [flags] <label>

<label> is the snapshot label.

The jiri snapshot create flags are:
 -push-remote=false
   Commit and push snapshot upstream.
 -time-format=2006-01-02T15:04:05Z07:00
   Time format for snapshot file name.

 -color=true
   Use color to format output.
 -dir=
   Directory where snapshot are stored.  Defaults to $JIRI_ROOT/.snapshot.
 -v=false
   Print verbose output.

Jiri snapshot list - List existing project snapshots

The "snapshot list" command lists existing snapshots of the labels specified as
command-line arguments. If no arguments are provided, the command lists
snapshots for all known labels.

Usage:
   jiri snapshot list [flags] <label ...>

<label ...> is a list of snapshot labels.

The jiri snapshot list flags are:
 -color=true
   Use color to format output.
 -dir=
   Directory where snapshot are stored.  Defaults to $JIRI_ROOT/.snapshot.
 -v=false
   Print verbose output.

Jiri update - Update all jiri tools and projects

Updates all projects, builds the latest version of all tools, and installs the
resulting binaries into $JIRI_ROOT/.jiri_root/bin. The sequence in which the
individual updates happen guarantees that we end up with a consistent set of
tools and source code. The set of projects and tools to update is described in
the manifest.

Run "jiri help manifest" for details on manifests.

Usage:
   jiri update [flags]

The jiri update flags are:
 -attempts=1
   Number of attempts before failing.
 -gc=false
   Garbage collect obsolete repositories.
 -manifest=
   Name of the project manifest.

 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri which - Show path to the jiri tool

Which behaves similarly to the unix commandline tool.  It is useful in
determining whether the jiri binary is being run directly, or run via the jiri
shim script.

If the binary is being run directly, the output looks like this:

  # binary
  /path/to/binary/jiri

If the script is being run, the output looks like this:

  # script
  /path/to/script/jiri

Usage:
   jiri which [flags]

The jiri which flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

Jiri runp - Run a command in parallel across jiri projects

Run a command in parallel across one or more jiri projects using the specified
profile target's environment. Commands are run using the shell specified by the
users $SHELL environment variable, or "sh" if that's not set. Thus commands are
run as $SHELL -c "args..."

Usage:
   jiri runp [flags] <command line>

A command line to be run in each project specified by the supplied command line
flags. Any environment variables intended to be evaluated when the command line
is run must be quoted to avoid expansion before being passed to runp by the
shell.

The jiri runp flags are:
 -collate-stdout=true
   Collate all stdout output from each parallel invocation and display it as if
   had been generated sequentially. This flag cannot be used with
   -show-name-prefix, -show-key-prefix or -interactive.
 -env=
   specify an environment variable in the form: <var>=[<val>],...
 -exit-on-error=false
   If set, all commands will killed as soon as one reports an error, otherwise,
   each will run to completion.
 -has-branch=
   A regular expression specifying branch names to use in matching projects. A
   project will match if the specified branch exists, even if it is not checked
   out.
 -has-gerrit-message=false
   If specified, match branches that have, or have no, gerrit message
 -has-uncommitted=false
   If specified, match projects that have, or have no, uncommitted changes
 -has-untracked=false
   If specified, match projects that have, or have no, untracked files
 -interactive=true
   If set, the command to be run is interactive and should not have its
   stdout/stderr manipulated. This flag cannot be used with -show-name-prefix,
   -show-key-prefix or -collate-stdout.
 -merge-policies=+CCFLAGS,+CGO_CFLAGS,+CGO_CXXFLAGS,+CGO_LDFLAGS,+CXXFLAGS,GOARCH,GOOS,GOPATH:,^GOROOT*,+LDFLAGS,:PATH,VDLPATH:
   specify policies for merging environment variables
 -profiles=v23:base,jiri
   a comma separated list of profiles to use
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -projects=
   A Regular expression specifying project keys to run commands in. By default,
   runp will use projects that have the same branch checked as the current
   project.
 -show-key-prefix=false
   If set, each line of output from each project will begin with the key of the
   project followed by a colon. This is intended for use with long running
   commands where the output needs to be streamed. Stdout and stderr are spliced
   apart. This flag cannot be used with -interactive, -show-name-prefix or
   -collate-stdout
 -show-name-prefix=false
   If set, each line of output from each project will begin with the name of the
   project followed by a colon. This is intended for use with long running
   commands where the output needs to be streamed. Stdout and stderr are spliced
   apart. This flag cannot be used with -interactive, -show-key-prefix or
   -collate-stdout.
 -skip-profiles=false
   if set, no profiles will be used
 -target=<runtime.GOARCH>-<runtime.GOOS>
   specifies a profile target in the following form: <arch>-<os>[@<version>]
 -v=false
   Print verbose logging information

 -color=true
   Use color to format output.

Jiri help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri help flags are:
 -style=compact
   The formatting style for help output:
      compact   - Good for compact cmdline output.
      full      - Good for cmdline output, shows all global flags.
      godoc     - Good for godoc processing.
      shortonly - Only output short description.
   Override the default by setting the CMDLINE_STYLE environment variable.
 -width=<terminal width>
   Format output to this target width in runes, or unlimited if width < 0.
   Defaults to the terminal width if available.  Override the default by setting
   the CMDLINE_WIDTH environment variable.

Jiri filesystem - Description of jiri file system layout

All data managed by the jiri tool is located in the file system under a root
directory, colloquially called the jiri root directory.  The file system layout
looks like this:

 [root]                              # root directory (name picked by user)
 [root]/.jiri_root                   # root metadata directory
 [root]/.jiri_root/bin               # contains tool binaries (jiri, etc.)
 [root]/.jiri_root/update_history    # contains history of update snapshots
 [root]/.manifest                    # contains jiri manifests
 [root]/[project1]                   # project directory (name picked by user)
 [root]/[project1]/.jiri             # project metadata directory
 [root]/[project1]/.jiri/metadata.v2 # project metadata file
 [root]/[project1]/.jiri/<<cls>>     # project per-cl metadata directories
 [root]/[project1]/<<files>>         # project files
 [root]/[project2]...

The [root] and [projectN] directory names are picked by the user.  The <<cls>>
are named via jiri cl new, and the <<files>> are named as the user adds files
and directories to their project.  All other names above have special meaning to
the jiri tool, and cannot be changed; you must ensure your path names don't
collide with these special names.

There are two ways to run the jiri tool:

1) Shim script (recommended approach).  This is a shell script that looks for
the [root] directory.  If the JIRI_ROOT environment variable is set, that is
assumed to be the [root] directory.  Otherwise the script looks for the
.jiri_root directory, starting in the current working directory and walking up
the directory chain.  The search is terminated successfully when the .jiri_root
directory is found; it fails after it reaches the root of the file system.  Thus
the shim must be invoked from the [root] directory or one of its subdirectories.

Once the [root] is found, the JIRI_ROOT environment variable is set to its
location, and [root]/.jiri_root/bin/jiri is invoked.  That file contains the
actual jiri binary.

The point of the shim script is to make it easy to use the jiri tool with
multiple [root] directories on your file system.  Keep in mind that when "jiri
update" is run, the jiri tool itself is automatically updated along with all
projects.  By using the shim script, you only need to remember to invoke the
jiri tool from within the appropriate [root] directory, and the projects and
tools under that [root] directory will be updated.

The shim script is located at [root]/release/go/src/v.io/jiri/scripts/jiri

2) Direct binary.  This is the jiri binary, containing all of the actual jiri
tool logic.  The binary requires the JIRI_ROOT environment variable to point to
the [root] directory.

Note that if you have multiple [root] directories on your file system, you must
remember to run the jiri binary corresponding to the setting of your JIRI_ROOT
environment variable.  Things may fail if you mix things up, since the jiri
binary is updated with each call to "jiri update", and you may encounter version
mismatches between the jiri binary and the various metadata files or other
logic.  This is the reason the shim script is recommended over running the
binary directly.

The jiri binary is located at [root]/.jiri_root/bin/jiri

Jiri manifest - Description of manifest files

Jiri manifest files describe the set of projects that get synced and tools that
get built when running "jiri update".

The first manifest file that jiri reads is in $JIRI_ROOT/.jiri_manifest.  This
manifest **must** exist for the jiri tool to work.

Usually the manifest in $JIRI_ROOT/.jiri_manifest will import other manifests
from remote repositories via <import> tags, but it can contain its own list of
projects and tools as well.

Manifests have the following XML schema:

<manifest>
  <imports>
    <import remote="https://vanadium.googlesource.com/manifest"
            manifest="public"
            name="manifest"
    />
    <localimport file="/path/to/local/manifest"/>
    ...
  </imports>
  <projects>
    <project name="my-project"
             path="path/where/project/lives"
             protocol="git"
             remote="https://github.com/myorg/foo"
             revision="ed42c05d8688ab23"
             remotebranch="my-branch"
             gerrithost="https://myorg-review.googlesource.com"
             githooks="path/to/githooks-dir"
             runhook="path/to/runhook-script"
    />
    ...
  </projects>
  <tools>
    <tool name="jiri"
          package="v.io/jiri"
          project="release.go.jiri"
    />
    ...
  </tools>
</manifest>

The <import> and <localimport> tags can be used to share common projects and
tools across multiple manifests.

A <localimport> tag should be used when the manifest being imported and the
importing manifest are both in the same repository, or when neither one is in a
repository.  The "file" attribute is the path to the manifest file being
imported.  It can be absolute, or relative to the importing manifest file.

If the manifest being imported and the importing manifest are in different
repositories then an <import> tag must be used, with the following attributes:

* remote (required) - The remote url of the repository containing the manifest
to be imported

* manifest (required) - The path of the manifest file to be imported, relative
to the repository root.

* name (optional) - The name of the project corresponding to the manifest
repository.  If your manifest contains a <project> with the same remote as the
manifest remote, then the "name" attribute of on the <import> tag should match
the "name" attribute on the <project>.  Otherwise, jiri will clone the manifest
repository on every update.

The <project> tags describe the projects to sync, and what state they should
sync to, accoring to the following attributes:

* name (required) - The name of the project.

* path (required) - The location where the project will be located, relative to
the jiri root.

* remote (required) - The remote url of the project repository.

* protocol (optional) - The protocol to use when cloning and syncing the repo.
Currently "git" is the default and only supported protocol.

* remotebranch (optional) - The remote branch that the project will sync to.
Defaults to "master".  The "remotebranch" attribute is ignored if "revision" is
specified.

* revision (optional) - The specific revision (usually a git SHA) that the
project will sync to.  If "revision" is  specified then the "remotebranch"
attribute is ignored.

* gerrithost (optional) - The url of the Gerrit host for the project.  If
specified, then running "jiri cl mail" will upload a CL to this Gerrit host.

* githooks (optional) - The path (relative to $JIRI_ROOT) of a directory
containing git hooks that will be installed in the projects .git/hooks directory
during each update.

* runhook (optional) - The path (relate to $JIRI_ROOT) of a script that will be
run during each update.

The <tool> tags describe the tools that will be compiled and installed in
$JIRI_ROOT/.jiri_root/bin after each update.  The tools must be written in go,
and are identified by their package name and the project that contains their
code.  They are configured via the following attributes:

* name (required) - The name of the binary that will be installed in
  JIRI_ROOT/.jiri_root/bin

* package (required) - The name of the Go package that will be passed to "go
  build".

* project (required) - The name of the project that contains the source code
  for the tool.
*/
package main
