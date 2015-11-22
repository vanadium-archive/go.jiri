// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package jiritest provides utilities for testing jiri functionality.
package jiritest

import (
	"os"
	"testing"

	"v.io/jiri/jiri"
	"v.io/jiri/tool"
)

// NewX is similar to jiri.NewX, but is meant for usage in a testing environment.
func NewX(t *testing.T) (*jiri.X, func()) {
	ctx := tool.NewDefaultContext()
	root, err := ctx.NewSeq().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	oldEnv := os.Getenv(jiri.RootEnv)
	if err := os.Setenv(jiri.RootEnv, root); err != nil {
		t.Fatalf("Setenv(%v) failed: %v", jiri.RootEnv, err)
	}
	cleanup := func() {
		os.Setenv(jiri.RootEnv, oldEnv)
		ctx.NewSeq().RemoveAll(root).Done()
	}
	return &jiri.X{Context: ctx, Root: root}, cleanup
}

// NewX_DeprecatedEnv relies on the deprecated JIRI_ROOT environment variable to
// set up a new jiri.X.  Tests relying on this function need to be updated to
// not rely on the environment variable.
func NewX_DeprecatedEnv(t *testing.T, opts *tool.ContextOpts) *jiri.X {
	root := os.Getenv(jiri.RootEnv)
	if root == "" {
		t.Fatalf("%v isn't set", jiri.RootEnv)
	}
	var ctx *tool.Context
	if opts != nil {
		ctx = tool.NewContext(*opts)
	} else {
		ctx = tool.NewDefaultContext()
	}
	return &jiri.X{Context: ctx, Root: root}
}
