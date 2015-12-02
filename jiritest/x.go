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
	oldRoot := os.Getenv(jiri.RootEnv)
	if err := os.Setenv(jiri.RootEnv, root); err != nil {
		t.Fatalf("Setenv(%q, %q) failed: %v", jiri.RootEnv, root, err)
	}
	cleanup := func() {
		if err := os.Setenv(jiri.RootEnv, oldRoot); err != nil {
			t.Fatalf("Setenv(%q, %q) failed: %v", jiri.RootEnv, oldRoot, err)
		}
		if err := ctx.NewSeq().RemoveAll(root).Done(); err != nil {
			t.Fatalf("RemoveAll(%q) failed: %v", root, err)
		}
	}
	return &jiri.X{Context: ctx, Root: root}, cleanup
}
