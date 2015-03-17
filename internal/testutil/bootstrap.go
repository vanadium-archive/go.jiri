package testutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/xunit"
)

const (
	numAttempts = 3
)

// vanadiumBootstrap runs a test of Vanadium bootstrapping.
func vanadiumBootstrap(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Create a new temporary VANADIUM_ROOT.
	oldRoot := os.Getenv("VANADIUM_ROOT")
	defer collect.Error(func() error { return os.Setenv("VANADIUM_ROOT", oldRoot) }, &e)
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, internalTestError{err, "TempDir"}
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)

	root := filepath.Join(tmpDir, "root")
	if err := os.Setenv("VANADIUM_ROOT", root); err != nil {
		return nil, internalTestError{err, "Setenv"}
	}

	// Run the setup script.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = io.MultiWriter(opts.Stdout, &out)
	opts.Stderr = io.MultiWriter(opts.Stderr, &out)
	opts.Env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Join(oldRoot, "bin"), "", -1)
	for i := 1; i <= numAttempts; i++ {
		if i > 1 {
			fmt.Fprintf(ctx.Stdout(), "Attempt %d/%d:\n", i, numAttempts)
		}
		if err = ctx.Run().CommandWithOpts(opts, filepath.Join(oldRoot, "scripts", "setup", "bootstrap")); err == nil {
			break
		}
	}
	if err != nil {
		// Create xUnit report.
		if err := xunit.CreateFailureReport(ctx, testName, "VanadiumGo", "bootstrap", "Vanadium bootstrapping failed", out.String()); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
