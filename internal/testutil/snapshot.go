// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"fmt"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
)

// vanadiumGoSnapshot create a snapshot of Vanadium Go code base.
func vanadiumGoSnapshot(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Create a new snapshot.
	const numAttempts = 3
	for i := 1; i <= numAttempts; i++ {
		if i > 1 {
			fmt.Fprintf(ctx.Stdout(), "Attempt %d/%d:\n", i, numAttempts)
		}
		if err = ctx.Run().Command("v23", "snapshot", "-remote", "create", "stable-go"); err == nil {
			return &TestResult{Status: TestPassed}, nil
		} else {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		}
	}
	return nil, internalTestError{err, "Snapshot"}
}
