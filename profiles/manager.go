// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profiles implements support for managing external sofware dependencies.
// It offers a balance between providing no support at all and a full blown package
// manager. A profile is a named collection of software required for a given
// system component or application. The name of the profile refers to all of the
// required software, which may a single library or a collection of libraries or
// SDKs. Profiles thus refer to uncompiled source code that needs to be compiled
// for a specific "target". Targets represent compiled code and consist of:
//
// 1. A 'tag' that can be used a short hand for refering to a target
// 2. An 'architecture' that refers to the CPU to be generate code for
// 3. An 'operating system' that refers to the operating system to generate
//    code for.
// 4. An 'environment' which is a set of environment variables to use when
//    compiling and using the profile.
//
// Targets provide the essential support for cross compilation.
//
// The profiles package provides a registry for profile implementations to
// register themselves (by calling profiles.Register from an init function
// for example) and for managing a 'manifest' of the currently built
// profiles. The manifest is represented as an XML file.
//
// Profiles may be installed, updated or removed. When doing so, the name of
// the profile is required, but the other components of the target are optional
// and will default to the values of the system that the commands are run on
// (so-called native builds). Once a profile is installed it may be referred to
// by its tag for subsequent updates and removals. These operations are
// defined by the profiles.Manager interface.
//
// The manifest tracks the installed profiles and their configurations.
// Other command line tools and packages are expected read information about
// the currently installed profiles from this manifest via profiles.ConfigHelper.
package profiles

import (
	"errors"
	"flag"
	"sort"
	"sync"

	"v.io/jiri/tool"
)

var (
	registry = struct {
		sync.Mutex
		managers map[string]Manager
	}{
		managers: make(map[string]Manager),
	}
)

// Register is used to register a profile manager. It is an error
// to call Registerr more than once with the same name, though it
// is possible to register the same Manager using different names.
func Register(name string, mgr Manager) {
	registry.Lock()
	defer registry.Unlock()
	if _, present := registry.managers[name]; present {
		panic("a profile manager is already registered for: " + name)
	}
	registry.managers[name] = mgr
}

// Managers returns the names, in lexicographic order, of all of the currently
// available profile managers.
func Managers() []string {
	registry.Lock()
	defer registry.Unlock()
	names := make([]string, 0, len(registry.managers))
	for name := range registry.managers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LookupManager returns the manager for the named profile or nil if one is
// not found.
func LookupManager(name string) Manager {
	registry.Lock()
	defer registry.Unlock()
	return registry.managers[name]
}

type Action int

const (
	Install Action = iota
	Update
	Uninstall
)

var ErrNoIncrementalUpdate = errors.New("incremental update is not supported")

// Manager is the interface that must be implemented in order to
// manage (i.e. install/uninstall/update) a profile.
type Manager interface {
	// AddFlags allows the profile manager to add profile specific flags
	// to the supplied FlagSet for the specified Action.
	// They should be named <profile-name>.<flag>.
	AddFlags(*flag.FlagSet, Action)
	// SetRoot sets the top level directory for the installation. It must be
	// called once and before Install/Uninstall/Update are called.
	SetRoot(dir string)
	// Root returns the top level directory for the installation.
	Root() string
	// Name returns the name of this profile.
	Name() string
	// String returns a string representation of the profile, conventionally this
	// is its name and version.
	String() string
	// Install installs the profile for the specified build target.
	Install(ctx *tool.Context, target Target) error
	// Uninstall uninstalls the profile for the specified build target. When
	// the last target for any given profile is uninstalled, then the profile
	// itself (i.e. the source code) will be uninstalled.
	Uninstall(ctx *tool.Context, target Target) error
	// Update updates the specified build target. Update may return
	// ErrNoIncrementalUpdate to its caller so that the caller may instead
	// opt to Uninstall and re-Install the profile in lieu of an incremental update.
	Update(ctx *tool.Context, target Target) error
}
