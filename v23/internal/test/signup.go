// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

func vanadiumSignupProxy(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	var buffer bytes.Buffer
	{
		credentials := os.Getenv("CREDENTIALS")
		fetchSrc := filepath.Join(root, "infrastructure", "signup", "fetch.go")
		opts := ctx.Run().Opts()
		opts.Stdout = &buffer
		if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", fetchSrc, "-credentials="+credentials); err != nil {
			return nil, internalTestError{err, "fetch"}
		}
	}

	// Create a feature branch in the infrastructure project.
	infraDir := tool.RootDirOpt(filepath.Join(root, "infrastructure"))
	if err := ctx.Git(infraDir).CreateAndCheckoutBranch("update"); err != nil {
		return nil, internalTestError{err, "create"}
	}
	defer collect.Error(func() error {
		if err := ctx.Git(infraDir).CheckoutBranch("master", gitutil.Force); err != nil {
			return internalTestError{err, "checkout"}
		}
		if err := ctx.Git(infraDir).DeleteBranch("update", gitutil.Force); err != nil {
			return internalTestError{err, "delete"}
		}
		return nil
	}, &e)

	// Update emails address whitelists.
	{
		whitelists := strings.Split(os.Getenv("WHITELISTS"), string(filepath.ListSeparator))
		mergeSrc := filepath.Join(root, "infrastructure", "signup", "merge.go")
		for _, whitelist := range whitelists {
			opts := ctx.Run().Opts()
			opts.Stdin = bytes.NewReader(buffer.Bytes())
			if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", mergeSrc, "-whitelist="+whitelist); err != nil {
				return nil, internalTestError{err, "merge"}
			}
			if err := ctx.Git(infraDir).Add(whitelist); err != nil {
				return nil, internalTestError{err, "commit"}
			}
		}
	}

	// Push changes (if any exist) to master.
	changed, err := ctx.Git(infraDir).HasUncommittedChanges()
	if err != nil {
		return nil, internalTestError{err, "changes"}
	}
	if changed {
		if err := ctx.Git(infraDir).CommitWithMessage("updating list of emails"); err != nil {
			return nil, internalTestError{err, "commit"}
		}
		if err := ctx.Git(infraDir).Push("origin", "update:master", !gitutil.Verify); err != nil {
			return nil, internalTestError{err, "push"}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}
