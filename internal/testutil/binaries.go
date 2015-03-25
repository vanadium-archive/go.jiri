// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

// vanadiumGoBinaries uploads Vanadium binaries to Google Storage.
func vanadiumGoBinaries(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Fetch the latest stable Go snapshot.
	if err := ctx.Run().Command("v23", "update", "-manifest=stable-go"); err != nil {
		return nil, internalTestError{err, "Update"}
	}

	// Build all v.io binaries.
	if err := ctx.Run().Command("v23", "go", "install", "v.io/..."); err != nil {
		return nil, internalTestError{err, "Install"}
	}

	// Upload all v.io binaries to Google Storage.
	timestamp := time.Now().Format(time.RFC3339)
	bucket := fmt.Sprintf("gs://vanadium-binaries/%s_%s/", runtime.GOOS, runtime.GOARCH)
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}
	binaries := filepath.Join(root, "release", "go", "bin", "*")
	if err := ctx.Run().Command("gsutil", "-m", "-q", "cp", binaries, bucket+timestamp); err != nil {
		return nil, internalTestError{err, "Upload"}
	}

	// Upload two files: 1) a file that identifies the directory
	// containing the latest set of binaries and 2) a file that
	// indicates that the upload of binaries succeeded.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, internalTestError{err, "TempDir"}
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
	doneFile := filepath.Join(tmpDir, ".done")
	if err := ctx.Run().WriteFile(doneFile, nil, os.FileMode(0600)); err != nil {
		return nil, internalTestError{err, "WriteFile"}
	}
	if err := ctx.Run().Command("gsutil", "-q", "cp", doneFile, bucket+timestamp); err != nil {
		return nil, internalTestError{err, "Upload"}
	}
	latestFile := filepath.Join(tmpDir, "latest")
	if err := ctx.Run().WriteFile(latestFile, []byte(timestamp), os.FileMode(0600)); err != nil {
		return nil, internalTestError{err, "WriteFile"}
	}
	if err := ctx.Run().Command("gsutil", "-q", "cp", latestFile, bucket); err != nil {
		return nil, internalTestError{err, "Upload"}
	}

	return &TestResult{Status: TestPassed}, nil
}
