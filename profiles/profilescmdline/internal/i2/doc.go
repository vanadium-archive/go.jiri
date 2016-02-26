// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

/*
Profiles are used to manage external sofware dependencies and offer a balance
between providing no support at all and a full blown package manager. Profiles
can be built natively as well as being cross compiled. A profile is a named
collection of software required for a given system component or application.
Current example profiles include 'syncbase' which consists of the leveldb and
snappy libraries or 'android' which consists of all of the android components
and downloads needed to build android applications. Profiles are built for
specific targets.

Targets

Profiles generally refer to uncompiled source code that needs to be compiled for
a specific "target". Targets hence represent compiled code and consist of:

1. An 'architecture' that refers to the CPU to be generate code for

2. An 'operating system' that refers to the operating system to generate code
for

3. A lexicographically orderd set of supported versions, one of which is
designated as the default.

4. An 'environment' which is a set of environment variables to use when
compiling the profile

Targets thus provide the basic support needed for cross compilation.

Targets are versioned and multiple versions may be installed and used
simultaneously. Versions are ordered lexicographically and each target specifies
a 'default' version to be used when a specific version is not explicitly
requested. A request to 'upgrade' the profile will result in the installation of
the default version of the targets currently installed if that default version
is not already installed.

The Supported Commands

Profiles, or more correctly, targets for specific profiles may be installed or
removed. When doing so, the name of the profile is required, but the other
components of the target are optional and will default to the values of the
system that the commands are run on (so-called native builds) and the default
version for that target. Once a profile is installed it may be referred to by
its tag for subsequent removals.

The are also update and cleanup commands. Update installs the default version of
the requested profile or for all profiles for the already installed targets.
Cleanup will uninstall targets whose version is older than the default.

Finally, there are commands to list the available and installed profiles and to
access the environment variables specified and stored in each profile
installation and a command (recreate) to generate a list of commands that can be
run to recreate the currently installed profiles.

The Profiles Database

The profiles packages manages a database that tracks the installed profiles and
their configurations. Other command line tools and packages are expected to read
information about the currently installed profiles from this database via the
profiles package. The profile command line tools support displaying the database
(via the list command) or for specifying an alternate version of the file (via
the -profiles-db flag) which is generally useful for debugging.

Adding Profiles

Profiles are intended to be provided as go packages that register themselves
with the profile command line tools via the *v.io/jiri/profiles* package. They
must implement the interfaces defined by that package and be imported (e.g.
import _ "myprofile") by the command line tools that are to use them.

Usage:
   jiri profile-i2 [flags] <command>

The jiri profile-i2 commands are:
   install     Install the given profiles
   uninstall   Uninstall the given profiles
   update      Install the latest default version of the given profiles
   cleanup     Cleanup the locally installed profiles
   available   List the available profiles
   help        Display help for commands or topics

The jiri profile-i2 flags are:
 -color=true
   Use color to format output.
 -v=false
   Print verbose output.

The global flags are:
 -metadata=<just specify -metadata to activate>
   Displays metadata for the program and exits.
 -time=false
   Dump timing information to stderr before exiting the program.

Jiri profile-i2 install - Install the given profiles

Install the given profiles.

Usage:
   jiri profile-i2 install [flags] <profiles>

<profiles> is a list of profiles to install.

The jiri profile-i2 install flags are:
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

Jiri profile-i2 uninstall - Uninstall the given profiles

Uninstall the given profiles.

Usage:
   jiri profile-i2 uninstall [flags] <profiles>

<profiles> is a list of profiles to uninstall.

The jiri profile-i2 uninstall flags are:
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

Jiri profile-i2 update - Install the latest default version of the given profiles

Install the latest default version of the given profiles.

Usage:
   jiri profile-i2 update [flags] <profiles>

<profiles> is a list of profiles to update, if omitted all profiles are updated.

The jiri profile-i2 update flags are:
 -profiles-db=$JIRI_ROOT/.jiri_root/profile_db
   the path, relative to JIRI_ROOT, that contains the profiles database.
 -profiles-dir=.jiri_root/profiles
   the directory, relative to JIRI_ROOT, that profiles are installed in
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile-i2 cleanup - Cleanup the locally installed profiles

Cleanup the locally installed profiles. This is generally required when
recovering from earlier bugs or when preparing for a subsequent change to the
profiles implementation.

Usage:
   jiri profile-i2 cleanup [flags] <profiles>

<profiles> is a list of profiles to cleanup, if omitted all profiles are
cleaned.

The jiri profile-i2 cleanup flags are:
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

Jiri profile-i2 available - List the available profiles

List the available profiles.

Usage:
   jiri profile-i2 available [flags]

The jiri profile-i2 available flags are:
 -describe=false
   print the profile description
 -v=false
   print more detailed information

 -color=true
   Use color to format output.

Jiri profile-i2 help - Display help for commands or topics

Help with no args displays the usage of the parent command.

Help with args displays the usage of the specified sub-command or help topic.

"help ..." recursively displays help for all commands and topics.

Usage:
   jiri profile-i2 help [flags] [command/topic ...]

[command/topic ...] optionally identifies a specific sub-command or help topic.

The jiri profile-i2 help flags are:
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
*/
package main
