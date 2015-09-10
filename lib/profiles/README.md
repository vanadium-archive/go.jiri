# Profiles

Profiles are used to manage external sofware dependencies and offer a balance
between providing no support at all and a full blown package manager.
A profile is a named collection of software required for a given system component or
application. The name of the profile refers to all of the required software,
which may a single library or a collection of libraries or SDKs.
Current example profiles include 'syncbase' which consists of the leveldb and
snappy libraries or 'android' which consists of all of the android components and
downloads needed to build android applications.

## Targets

Profiles generally refer to uncompiled source code that needs to be compiled for
a specific "target". Targets hence represent compiled code and consist of:

1. A 'tag' that can be used a short hand for refering to a target
2. An 'architecture' that refers to the CPU to be generate code for
3. An 'operating system' that refers to the operating system to generate code for
4. An 'environment' which is a set of environment variables to use when compiling the profile

Targets thus provide the basic support needed for cross compilation.

## The Supported Commands

Profiles may be installed, updated or removed. When doing so, the name of the
profile is required, but the other components of the target are optional and will
default to the values of the system that the commands are run on (so-called
native builds). Once a profile is installed it may be referred to by its tag
for subsequent updates and removals.

## Adding Profiles

Profiles are intended to be provided as go packages that register themselves
with the profile command line tools via the *v.io/jiri/lib/profiles* package.
They must implement the interfaces defined by that package and be imported
(e.g. import _ "myprofile") by the command line tools that are to use them.

## The Manifest

The *profiles* package manages a manifest that tracks the installed profiles
and their configurations. Other command line tools and packages are expected
to read information about the currently installed profiles from this manifest
via the *profiles* package.

