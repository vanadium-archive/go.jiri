// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profiles implements support for managing suites of software that
// are required by particular applications. Such suites are called 'profiles'
// and are managed by any package that implements the profiles.Manager interface.
// Profiles essentially represent 'code' that is to be configured and compiled
// for specified targets (profiles.Target). A single profile may be built
// for multiple targets and in particular cross-compilation may be so implemented.
// The common case is to build a profile for the target that represents the
// same host that is built on (native compilation).
//
// The profiles package provides a registry for profile implementations to
// register themselves (by calling profiles.Register from an init function
// for example) and for managing a 'manifest' of the currently built
// profiles. The manifest is represented as an XML file.

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
