// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jiri

import (
	"path/filepath"
	"strings"

	"v.io/x/lib/envvar"
)

// RelPath represents a relative path whose root is JIRI_ROOT.
type RelPath string

// NewRelPath returns a RelPath with path consisting of the specified
// components.
func NewRelPath(components ...string) RelPath {
	return RelPath(filepath.Join(components...))
}

// Abs returns an absolute path corresponding to the RelPath rooted at the
// JIRI_ROOT in X.
func (rp RelPath) Abs(x *X) string {
	return filepath.Join(x.Root, string(rp))
}

// Join returns a copy of RelPath with the specified components appended
// to the path using filepath.Join.
func (rp RelPath) Join(components ...string) RelPath {
	path := append([]string{string(rp)}, components...)
	return RelPath(filepath.Join(path...))
}

// Symbolic returns an absolute path corresponding to the RelPath, but
// with the JIRI_ROOT environment varible at the root instead of the actual
// value of JIRI_ROOT.
func (rp RelPath) Symbolic() string {
	root := "${" + RootEnv + "}"
	if string(rp) == "" {
		return root
	}
	return root + string(filepath.Separator) + string(rp)
}

// ExpandEnv expands all instances of the JIRI_ROOT variable in the supplied
// environment with the root from jirix.
func ExpandEnv(x *X, env *envvar.Vars) {
	e := env.ToMap()
	rootEnv := "${" + RootEnv + "}"
	for k, v := range e {
		n := strings.Replace(v, rootEnv, x.Root, -1)
		if n != v {
			env.Set(k, n)
		}
	}
}
