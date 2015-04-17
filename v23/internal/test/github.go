// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/internal/xunit"
)

var (
	mirrors = []Mirror{
		Mirror{
			name:         "environment",
			googlesource: "https://vanadium.googlesource.com/environment",
			github:       "git@github.com:vanadium/environment.git",
		},
		Mirror{
			name:         "browser",
			googlesource: "https://vanadium.googlesource.com/release.projects.browser",
			github:       "git@github.com:vanadium/browser.git",
		},
		Mirror{
			name:         "go.v23",
			googlesource: "https://vanadium.googlesource.com/release.go.v23",
			github:       "git@github.com:vanadium/go.v23.git",
		},
		Mirror{
			name:         "go.devtools",
			googlesource: "https://vanadium.googlesource.com/release.go.x.devtools",
			github:       "git@github.com:vanadium/go.devtools.git",
		},
		Mirror{
			name:         "go.lib",
			googlesource: "https://vanadium.googlesource.com/release.go.x.lib",
			github:       "git@github.com:vanadium/go.lib.git",
		},
		Mirror{
			name:         "go.ref",
			googlesource: "https://vanadium.googlesource.com/release.go.x.ref",
			github:       "git@github.com:vanadium/go.ref.git",
		},
		Mirror{
			name:         "js",
			googlesource: "https://vanadium.googlesource.com/release.js.core",
			github:       "git@github.com:vanadium/js.git",
		},
		Mirror{
			name:         "chat",
			googlesource: "https://vanadium.googlesource.com/release.projects.chat",
			github:       "git@github.com:vanadium/chat.git",
		},
		Mirror{
			name:         "pipe2browser",
			googlesource: "https://vanadium.googlesource.com/release.projects.pipe2browser",
			github:       "git@github.com:vanadium/pipe2browser.git",
		},
		Mirror{
			name:         "playground",
			googlesource: "https://vanadium.googlesource.com/release.projects.playground",
			github:       "git@github.com:vanadium/playground.git",
		},
		Mirror{
			name:         "scripts",
			googlesource: "https://vanadium.googlesource.com/scripts",
			github:       "git@github.com:vanadium/scripts.git",
		},
		Mirror{
			name:         "third_party",
			googlesource: "https://vanadium.googlesource.com/third_party",
			github:       "git@github.com:vanadium/third_party.git",
		},
		Mirror{
			name:         "www",
			googlesource: "https://vanadium.googlesource.com/www",
			github:       "git@github.com:vanadium/www.git",
		},
	}
)

type Mirror struct {
	name, googlesource, github string
}

// vanadiumGitHubMirror mirrors googlesource.com vanadium projects to
// github.com.
func vanadiumGitHubMirror(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test/task.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "V23Root"}
	}

	projects := filepath.Join(root, "projects")
	mode := os.FileMode(0755)
	if err := ctx.Run().MkdirAll(projects, mode); err != nil {
		return nil, internalTestError{err, "MkdirAll"}
	}

	allPassed := true
	suites := []xunit.TestSuite{}
	for _, mirror := range mirrors {
		suite, err := sync(ctx, mirror, projects)
		if err != nil {
			return nil, internalTestError{err, "sync"}
		}

		allPassed = allPassed && (suite.Failures == 0)
		suites = append(suites, *suite)
	}

	if err := xunit.CreateReport(ctx, testName, suites); err != nil {
		return nil, err
	}

	if !allPassed {
		return &test.Result{Status: test.Failed}, nil
	}

	return &test.Result{Status: test.Passed}, nil
}

func sync(ctx *tool.Context, mirror Mirror, projects string) (*xunit.TestSuite, error) {
	suite := xunit.TestSuite{Name: mirror.name}
	dirname := filepath.Join(projects, mirror.name)

	// If dirname does not exist `git clone` otherwise `git pull`.
	if _, err := os.Stat(dirname); err != nil {
		if !os.IsNotExist(err) {
			return nil, internalTestError{err, "Stat"}
		}

		err := clone(ctx, mirror, projects)
		testCase := makeTestCase("clone", err)
		if err != nil {
			suite.Failures++
		}
		suite.Cases = append(suite.Cases, *testCase)
	} else {
		err := pull(ctx, mirror, projects)
		testCase := makeTestCase("pull", err)
		if err != nil {
			suite.Failures++
		}
		suite.Cases = append(suite.Cases, *testCase)
	}

	err := push(ctx, mirror, projects)
	testCase := makeTestCase("push", err)
	if err != nil {
		suite.Failures++
	}
	suite.Cases = append(suite.Cases, *testCase)

	return &suite, nil
}

func makeTestCase(action string, err error) *xunit.TestCase {
	c := xunit.TestCase{
		Classname: "git",
		Name:      action,
	}

	if err != nil {
		f := xunit.Failure{
			Message: "git error",
			Data:    fmt.Sprintf("%v", err),
		}
		c.Failures = append(c.Failures, f)
	}

	return &c
}

func clone(ctx *tool.Context, mirror Mirror, projects string) error {
	dirname := filepath.Join(projects, mirror.name)
	return ctx.Git().Clone(mirror.googlesource, dirname)
}

func pull(ctx *tool.Context, mirror Mirror, projects string) error {
	dirname := filepath.Join(projects, mirror.name)
	opts := tool.RootDirOpt(dirname)
	return ctx.Git(opts).Pull("origin", "master")
}

func push(ctx *tool.Context, mirror Mirror, projects string) error {
	dirname := filepath.Join(projects, mirror.name)
	opts := tool.RootDirOpt(dirname)
	return ctx.Git(opts).Push(mirror.github, "master")
}
