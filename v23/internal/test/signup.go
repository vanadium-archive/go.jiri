// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/project"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
)

func vanadiumSignupProxy(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(ctx, "old_schema.go", testName)
}

func vanadiumSignupProxyNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupProxyHelper(ctx, "new_schema.go", testName)
}

func vanadiumSignupProxyHelper(ctx *tool.Context, schema, testName string) (_ *test.Result, e error) {
	root, err := project.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", schema, sheetID, false)
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

func vanadiumSignupWelcomeStepOneNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := project.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", "new_schema.go", sheetID, false)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	var emails bytes.Buffer

	welcome := filepath.Join(root, "infrastructure", "signup", "welcome.go")
	welcomeOpts := ctx.Run().Opts()
	welcomeOpts.Stdin = bytes.NewReader(data)
	welcomeOpts.Stdout = &emails
	sentlist := filepath.Join(root, "infrastructure", "signup", "sentlist.json")
	if err := ctx.Run().CommandWithOpts(welcomeOpts, "v23", "go", "run", welcome, "-sentlist="+sentlist); err != nil {
		return nil, internalTestError{err, "welcome"}
	}

	// Convert the newline delimited output from the command above into a slice of
	// strings which can be written to a file in the format:
	//
	//   EMAILS = <email> <email> <email...>
	//
	output := []string{"EMAILS", "="}
	reader := bytes.NewReader(emails.Bytes())
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		email := scanner.Text()
		output = append(output, email)
	}

	if err := scanner.Err(); err != nil {
		return nil, internalTestError{err, "Scan"}
	}

	// Join the array and convert it to bytes
	contents := strings.Join(output, " ")
	filename := filepath.Join(root, ".vanadium_signup_weclome_properties")

	if err := ctx.Run().WriteFile(filename, []byte(contents), 0644); err != nil {
		return nil, internalTestError{err, "WriteFile"}
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

	if err := ctx.Git(infraDir).Add(sentlist); err != nil {
		return nil, internalTestError{err, "commit"}
	}

	// Push changes (if any exist) to master.
	changed, err := ctx.Git(infraDir).HasUncommittedChanges()
	if err != nil {
		return nil, internalTestError{err, "changes"}
	}
	if changed {
		if err := ctx.Git(infraDir).CommitWithMessage("infrastructure/signup: updating sentlist"); err != nil {
			return nil, internalTestError{err, "commit"}
		}
		if err := ctx.Git(infraDir).Push("origin", "update:master", gitutil.VerifyOpt(false)); err != nil {
			return nil, internalTestError{err, "push"}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupWelcomeStepTwoNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := project.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	mailer := filepath.Join(root, "release", "go", "src", "v.io", "x", "devtools", "mailer", "mailer.go")
	if err := ctx.Run().Command("v23", "go", "run", mailer); err != nil {
		return nil, internalTestError{err, "mailer"}
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
	root, err := project.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "github", schema, sheetID, false)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Add them to @vanadium/developers
	githubToken := os.Getenv("GITHUB_TOKEN")
	github := filepath.Join(root, "infrastructure", "signup", "github.go")
	githubOpts := ctx.Run().Opts()
	githubOpts.Stdin = bytes.NewReader(data)
	if err := ctx.Run().CommandWithOpts(githubOpts, "v23", "go", "run", github, "-token="+githubToken); err != nil {
		return nil, internalTestError{err, "github"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumSignupGroup(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "old_schema.go", testName, false)
}

func vanadiumSignupGroupNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "new_schema.go", testName, false)
}

func vanadiumSignupDiscussNew(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumSignupGroupHelper(ctx, "new_schema.go", testName, true)
}

func vanadiumSignupGroupHelper(ctx *tool.Context, schema, testName string, discussOnly bool) (_ *test.Result, e error) {
	root, err := project.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	// Fetch emails addresses.
	credentials := os.Getenv("CREDENTIALS")
	sheetID := os.Getenv("SHEET_ID")

	data, err := fetchFieldValues(ctx, credentials, "email", schema, sheetID, discussOnly)
	if err != nil {
		return nil, internalTestError{err, "fetch"}
	}

	// Add them to Google Group.
	keyFile := os.Getenv("KEYFILE")
	serviceAccount := os.Getenv("SERVICE_ACCOUNT")
	groupEmail := os.Getenv("GROUP_EMAIL")
	opts := ctx.Run().Opts()
	opts.Stdin = bytes.NewReader(data)
	groupSrc := filepath.Join(root, "infrastructure", "signup", "group.go")
	if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "run", groupSrc, "-keyFile="+keyFile, "-account="+serviceAccount, "-groupEmail="+groupEmail); err != nil {
		return nil, internalTestError{err, "group"}
	}

	return &test.Result{Status: test.Passed}, nil
}

func fetchFieldValues(ctx *tool.Context, credentials, field, schema, sheetID string, discussOnly bool) ([]byte, error) {
	root, err := project.V23Root()
	if err != nil {
		return nil, internalTestError{err, "VanadiumRoot"}
	}

	var buffer bytes.Buffer

	fetchSrc := filepath.Join(root, "infrastructure", "signup", "fetch.go")
	schemaSrc := filepath.Join(root, "infrastructure", "signup", schema)
	opts := ctx.Run().Opts()
	opts.Stdout = &buffer
	args := []string{"go", "run", fetchSrc, schemaSrc, "-credentials="+credentials, "-field="+field, "-sheet-id="+sheetID}
	if discussOnly {
	  args = append(args, "-discuss-only")
	}
	if err := ctx.Run().CommandWithOpts(opts, "v23", args...); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
