package testutil

import (
	"path/filepath"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
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

	// Build all v.io binaries.
	if err := ctx.Run().Command("v23", "go", "install", "v.io/..."); err != nil {
		return nil, internalTestError{err, "Install"}
	}

	// Upload binaries used for monitoring to Google Storage.
	//
	// TODO(jingjin): If we are still using gs://veyron-monitoring,
	// replace it with gs://vanadium-monitoring. Otherwise, just get rid
	// of this code.
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}
	debugBin := filepath.Join(root, "release", "go", "bin", "debug")
	vrpcBin := filepath.Join(root, "release", "go", "bin", "vrpc")
	if err := ctx.Run().Command("gsutil", "-q", "cp", debugBin, vrpcBin, "gs://veyron-monitoring/bin/"); err != nil {
		return nil, internalTestError{err, "Upload"}
	}

	return &TestResult{Status: TestPassed}, nil
}
