// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profiles and its subdirectoris implement support for managing external
// sofware dependencies. They offer a balance between providing no support at
// all and a full blown package manager. A profile is a named collection of
// software required for a given system component or application. The name of
// the profile refers to all of the required software, which may a single
// library or a collection of libraries or SDKs. Profiles thus refer to
// uncompiled source code that needs to be compiled for a specific "target".
// Targets represent compiled code and consist of:
//
// 1. An 'architecture' that refers to the CPU to be generate code for.
//
// 2. An 'operating system' that refers to the operating system to generate
// code for.
//
// 3. An 'environment' which is a set of environment variables to use when
// compiling and using the profile.
//
// Targets provide the essential support for cross compilation.
//
// The profiles package provides the data types to support its sub-packages,
// including a database format (in XML) that is used to store the state of
// the currently installed profiles.
//
// The profilesmanager sub-package provides a registry for profile
// implementations to register themselves (by calling manager.Register
// from an init function for example).
//
// The profilesreader sub-package provides support for reading the profiles
// database and performing common operations on it.
// profiles/commandline provides an easy to use command line environment for
// tools that need to read and/or write profiles.
//
// The profilescmdline sub-package provides support for implementing
// command line clients that implement profile installation and for
// reading profile information. It also provides support for invoking
// sub-commands to manage families of profiles which uses the notion
// of a 'qualified' profile name where the profile name includes a prefix
// (separated by InstallSeparator) which identifies the sub-command responsible
// for managing that profile.
//
// The profilesutil sub-package provides some utility routines intended
// for use by profile implementers.
//
// Profiles may be installed, updated or removed. When doing so, the name of
// the profile is required, but the other components of the target are optional
// and will default to the values of the system that the commands are run on
// (so-called native builds). These operations are defined by the
// profiles/manager.Manager interface.
package profiles

import (
	"flag"
	"strings"

	"v.io/jiri"
)

// InstallerSeparator is the string used to separate the installer
// and profile name for qualified profile names.
const InstallerSeparator = ":"

// Profile represents an installed profile and its associated targets.
type Profile struct {
	name, root, installer string
	targets               Targets
}

func clean(n string) string {
	return strings.TrimRight(strings.TrimLeft(n, InstallerSeparator), InstallerSeparator)
}

// SplitProfileName returns the installer and profile name in the
// case where the name is qualified (see QualifiedProfileName) or just
// the profile name if it is not so qualified.
func SplitProfileName(name string) (string, string) {
	if pos := strings.Index(name, InstallerSeparator); pos >= 0 {
		return name[:pos], name[pos+len(InstallerSeparator):]
	}
	return "", name
}

// QualifiedProfileName returns the name of a profile qualified by
// the name of its installer, if any. If the installer is "myproject"
// and the profile name is "bar" then then returned string is
// "myproject" + InstallerSeparator + "bar". If the installer is not
// specified then the unqualified name, without InstallerSeparator is returned.
// If the supplied name is already qualified then the existing qualifier
// will be removed and the supplied one used instead.
// Any leading colons in name will be stripped.
func QualifiedProfileName(installer, name string) string {
	name = clean(name)
	installer = clean(installer)
	if installer != "" {
		// Strip any existing installer.
		_, n := SplitProfileName(name)
		return installer + InstallerSeparator + n
	}
	return name
}

// Name returns the qualified name of this profile.
func (p *Profile) Name() string {
	return QualifiedProfileName(p.installer, p.name)
}

// Root returns the directory, relative to the jiri root, that this
// profile is installed at.
func (p *Profile) Root() string {
	return p.root
}

// Targets returns the currently installed set of targets for this profile.
// Note that Targets is ordered by architecture, operating system and
// descending versions.
func (p *Profile) Targets() Targets {
	r := make(Targets, len(p.targets), len(p.targets))
	for i, t := range p.targets {
		tmp := *t
		r[i] = &tmp
	}
	return r
}

type Action int

const (
	Install Action = iota
	Uninstall
)

// Manager is the interface that must be implemented in order to
// manage (i.e. install/uninstall) and describe a profile.
type Manager interface {
	// Name returns the unqualified name of this profile.
	Name() string

	// Installer returns the installer for this profile.
	Installer() string

	// Info returns an informative description of the profile.
	Info() string

	// VersionInfo returns the VersionInfo instance for this profile.
	VersionInfo() *VersionInfo

	// String returns a string representation of the profile, conventionally
	// this is its qualified name and version.
	String() string

	// AddFlags allows the profile manager to add profile specific flags
	// to the supplied FlagSet for the specified Action.
	// They should be named <profile-name>.<flag>.
	AddFlags(*flag.FlagSet, Action)

	// OSPackages returns the set of operating system packages required by the
	// specified profile and target.
	OSPackages(jirix *jiri.X, pdb *DB, root jiri.RelPath, target Target) ([]string, error)

	// Install installs the profile for the specified build target.
	Install(jirix *jiri.X, pdb *DB, root jiri.RelPath, target Target) error

	// Uninstall uninstalls the profile for the specified build target.
	Uninstall(jirix *jiri.X, pdb *DB, root jiri.RelPath, target Target) error
}
