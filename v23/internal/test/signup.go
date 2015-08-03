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
	return vanadiumSignupProxyHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupProxyNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupProxyHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", schema, sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Create a feature branch in the infrastructure project.
	infraDir := tool.RootDirOpt(filepath.Join(root, "infrastructure"))
	if err := ctx.Git(infraDir).CreateAndCheckoutBranch("update"); err != nil {
		return nil, internalTestError{err, "create"}
	}
	defer collect.Error(func() error {
		if err := ctx.Git(infraDir).CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return internalTestError{err, "checkout"}
		}
		if err := ctx.Git(infraDir).DeleteBranch("update", gitutil.ForceOpt(true)); err != nil {
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
			opts.Stdin = bytes.NewReader(data)
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
		if err := ctx.Git(infraDir).Push("origin", "update:master", gitutil.VerifyOpt(false)); err != nil {
			return nil, internalTestError{err, "push"}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGithub(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGithubHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupGithubNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGithubHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupGithubHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "github", schema, sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Add them to @vanadium/developers
	github := filepath.Join(root, "infrastructure", "signup", "github.go")
	githubOpts := ctx.Run().Opts()
	githubOpts.Stdin = bytes.NewReader(data)
	if err := ctx.Run().CommandWithOpts(githubOpts, "v23", "go", "run", github, "-credentials="+credentials); err != nil {
		return nil, internalTestError{err, "github"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGroup(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupGroupNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupGroupHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", schema, sheetID)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Add them to Google Group.
	keyFile := os.Getenv("KEYFILE")
	serviceAccount := os.Getenv("SERVICE_ACCOUNT")
	opts := ctx.Run().Opts()
	opts.Stdin = bytes.NewReader(data)
	groupSrc := filepath.Join(root, "infrastructure", "signup", "group.go")
	if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", groupSrc, "-keyFile="+keyFile, "-account="+serviceAccount); err != nil {
		return nil, internalTestError{err, "group"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func fetchFieldValues(ctx *tool.Context, credentials, field, schema, sheetID string) ([]byte, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	var buffer bytes.Buffer

	fetchSrc := filepath.Join(root, "infrastructure", "signup", "fetch.go")
	schemaSrc := filepath.Join(root, "infrastructure", "signup", schema)
	opts := ctx.Run().Opts()
	opts.Stdout = &buffer
	if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", fetchSrc, schemaSrc, "-credentials="+credentials, "-field="+field, "-sheet-id="+sheetID); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
