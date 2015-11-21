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
	oldEnv := os.Getenv("JIRI_ROOT")
	if err := os.Setenv("JIRI_ROOT", root); err != nil {
		t.Fatalf("Setenv(JIRI_ROOT) failed: %v", err)
	}
	cleanup := func() {
		os.Setenv("JIRI_ROOT", oldEnv)
		ctx.NewSeq().RemoveAll(root).Done()
	}
	return &jiri.X{Context: ctx, Root: root}, cleanup
}
