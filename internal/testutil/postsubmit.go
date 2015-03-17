package testutil

import (
	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
)

// vanadiumPostsubmitPoll polls for new changes in all projects' master branches,
// and starts the corresponding Jenkins targets based on the changes.
func vanadiumPostsubmitPoll(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run the "postsubmit poll" command.
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"poll",
		"-manifest", "mirror/public",
	)
	if err := ctx.Run().Command("postsubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}
