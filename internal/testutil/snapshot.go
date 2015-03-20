package testutil

import (
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
	if err := ctx.Run().Command("v23", "snapshot", "-remote", "create", "stable-go"); err != nil {
		return nil, internalTestError{err, "Snapshot"}
	}

	return &TestResult{Status: TestPassed}, nil
}
