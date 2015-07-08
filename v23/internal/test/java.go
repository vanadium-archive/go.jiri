// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/envvar"
)

// vanadiumJavaTest runs all Java tests.
func vanadiumJavaTest(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"java"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	env := envvar.VarsFromOS()
	// Set JAVA_HOME environment variable, if not already set.
	if os.Getenv("JAVA_HOME") == "" {
		fmt.Println("JAVA_HOME not set, attempting to find a valid JDK...")
		var jdkLoc string
		var err error
		switch runtime.GOOS {
		case "linux":
			if jdkLoc, err = getJDKLinux(ctx); err != nil {
				return nil, err
			}
		case "darwin":
			if jdkLoc, err = getJDKDarwin(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("Unsupported operating system: %s", runtime.GOOS)
		}
		fmt.Println("Using: ", jdkLoc)
		env.Set("JAVA_HOME", jdkLoc)
	}

	// Run tests.
	rootDir, err := util.V23Root()
	javaDir := filepath.Join(rootDir, "release", "java")
	if err := ctx.Run().Chdir(javaDir); err != nil {
		return nil, err
	}
	runOpts := ctx.Run().Opts()
	runOpts.Env = env.ToMap()
	if err := ctx.Run().CommandWithOpts(runOpts, filepath.Join(javaDir, "gradlew"), ":lib:test"); err != nil {
		return nil, err
	}
	return &test.Result{Status: test.Passed}, nil
}

func getJDKLinux(ctx *tool.Context) (string, error) {
	javacBin := "/usr/bin/javac"
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	ctx.Run().CommandWithOpts(opts, "readlink", "-f", javacBin)
	if out.Len() == 0 {
		return "", errors.New("Couldn't find a valid Java installation: did you run \"v23 profile install java\"?")
	}
	// Strip "/bin/javac" from the returned path.
	return strings.TrimSuffix(out.String(), "/bin/javac\n"), nil
}

func getJDKDarwin(ctx *tool.Context) (string, error) {
	javaHomeBin := "/usr/libexec/java_home"
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	ctx.Run().CommandWithOpts(opts, javaHomeBin, "-t", "CommandLine", "-v", "1.7+")
	if out.Len() == 0 {
		return "", errors.New("Couldn't find a valid Java installation: did you run \"v23 profile install java\"?")
	}
	jdkLoc, _, err := bufio.NewReader(strings.NewReader(out.String())).ReadLine()
	if err != nil {
		return "", fmt.Errorf("Couldn't find a valid Java installation: %v", err)
	}
	return string(jdkLoc), nil
}
