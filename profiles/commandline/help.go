// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package commandline

var helpMsg = `
Profiles are used to manage external sofware dependencies and offer a balance
between providing no support at all and a full blown package manager.
Profiles can be built natively as well as being cross compiled.
A profile is a named collection of software required for a given system component or
application. Current example profiles include 'syncbase' which consists
of the leveldb and snappy libraries or 'android' which consists of all of the
android components and downloads needed to build android applications. Profiles
are built for specific targets.

Targets

Profiles generally refer to uncompiled source code that needs to be compiled for
a specific "target". Targets hence represent compiled code and consist of:

1. A 'tag' that can be used a short hand for refering to a target

2. An 'architecture' that refers to the CPU to be generate code for

3. An 'operating system' that refers to the operating system to generate code for

4. A lexicographically orderd set of supported versions, one of which is designated
as the default.

5. An 'environment' which is a set of environment variables to use when compiling the profile

Targets thus provide the basic support needed for cross compilation.

Targets are versioned and multiple versions may be installed and used simultaneously.
Versions are ordered lexicographically and each target specifies a 'default'
version to be used when a specific version is not explicitly requested. A request
to 'upgrade' the profile will result in the installation of the default version
of the targets currently installed if that default version is not already installed.


The Supported Commands

Profiles, or more correctly, targets for specific profiles may be installed or
removed. When doing so, the name of the profile is required, but the other
components of the target are optional and will default to the values of the
system that the commands are run on (so-called native builds) and the default
version for that target. Once a profile is installed it may be referred to by
its tag for subsequent removals.

The are also update and cleanup commands. Update installs the default version
of the requested profile or for all profiles for the already installed targets.
Cleanup will uninstall targets whose version is older than the default.

Finally, there are commands to list the available and installed profiles and
to access the environment variables specified and stored in each profile
installation and a command (recreate) to generate a list of commands that
can be run to recreate the currently installed profiles.

The Manifest

The profiles packages manages a manifest that tracks the installed profiles
and their configurations. Other command line tools and packages are expected
to read information about the currently installed profiles from this manifest
via the profiles package. The profile command line tools support displaying the
manifest (via the list command) or for specifying an alternate version of the
file (via the -manifest flag) which is generally useful for debugging.

Adding Profiles

Profiles are intended to be provided as go packages that register themselves
with the profile command line tools via the *v.io/jiri/profiles* package.
They must implement the interfaces defined by that package and be imported
(e.g. import _ "myprofile") by the command line tools that are to use them.
`
