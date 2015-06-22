// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

// vanadiumJavaTest runs all Java tests.
func vanadiumJavaTest(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"java"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	rootDir, err := util.V23Root()
	javaDir := filepath.Join(rootDir, "release", "java")
	if err := ctx.Run().Chdir(javaDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().Command(filepath.Join(javaDir, "gradlew"), ":lib:test"); err != nil {
		return nil, err
	}
	return &test.Result{Status: test.Passed}, nil
}
