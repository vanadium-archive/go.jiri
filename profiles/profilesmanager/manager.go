// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profilesmanager provides support for managing jiri profiles.
// In particular for installing and uninstalling them. It provides a
// registration mechanism for profile implementations to call from an init
// function to add themselves to the suite profiles available within this
// application.
package profilesmanager

import (
	"sort"
	"sync"

	"v.io/jiri/profiles"
)

var (
	registry = struct {
		sync.Mutex
		managers map[string]profiles.Manager
	}{
		managers: make(map[string]profiles.Manager),
	}
)

// Register is used to register a profile manager. It is an error
// to call Registerr more than once with the same name, though it
// is possible to register the same Manager using different names.
func Register(mgr profiles.Manager) {
	registry.Lock()
	defer registry.Unlock()
	qualifiedName := profiles.QualifiedProfileName(mgr.Installer(), mgr.Name())
	if _, present := registry.managers[qualifiedName]; present {
		panic("a profile manager is already registered for: " + qualifiedName)
	}
	registry.managers[qualifiedName] = mgr
}

// Names returns the names, in lexicographic order, of all of the currently
// available in-process, profile managers.
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

// LookupManager returns the manager for the named, in-process, profile or nil
// if one is not found.
func LookupManager(name string) profiles.Manager {
	registry.Lock()
	defer registry.Unlock()
	return registry.managers[name]
}
