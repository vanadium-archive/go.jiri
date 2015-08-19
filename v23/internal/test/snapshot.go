// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/retry"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
)

// vanadiumGoSnapshot create a snapshot of Vanadium Go code base.
func vanadiumGoSnapshot(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Create a new snapshot.
	fn := func() error {
		return ctx.Run().Command("v23", "snapshot", "-remote", "create", "stable-go")
	}
	if err := retry.Function(ctx, fn); err != nil {
		return nil, internalTestError{err, "Snapshot"}
	}
	return &test.Result{Status: test.Passed}, nil
}
