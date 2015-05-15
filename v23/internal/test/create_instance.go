// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	scriptEnvVar        = "CREATE_INSTANCE_SCRIPT"
	testInstancePrefix  = "create-instance-test"
	testInstanceProject = "google.com:veyron"
	testInstanceZone    = "us-central1-c"
)

var (
	defaultCreateInstanceTimeout = time.Minute * 10
	defaultCheckInstanceTimeout  = time.Minute * 5
)

type instance struct {
	Name              string
	Zone              string
	NetworkInterfaces []struct {
		AccessConfigs []struct {
			NatIP string
		}
	}
}

// vanadiumCreateInstanceTest creates a test instance using the
// create_instance.sh script (specified in the CREATE_INSTANCE_SCRIPT
// environment variable) and run prod service test and load test againest it.
func vanadiumCreateInstanceTest(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	// Check CREATE_INSTANCE_SCRIPT environment variable.
	script := os.Getenv(scriptEnvVar)
	if script == "" {
		return nil, internalTestError{fmt.Errorf("script not defined in %s environment variable", scriptEnvVar), "Env"}
	}

	cleanup, err := initTest(ctx, testName, []string{})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Clean up test instances possibly left by previous test runs.
	if err := cleanupTestInstances(ctx); err != nil {
		return nil, internalTestError{err, "Delete old test instances"}
	}

	// Run script.
	printBanner(ctx, fmt.Sprintf("Running instance creation script: %s", script))
	instanceName := fmt.Sprintf("%s-%s", testInstancePrefix, time.Now().Format("20060102150405"))
	defer collect.Error(func() error { return cleanupTestInstances(ctx) }, &e)
	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
		<-sigchan
		if err := cleanupTestInstances(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		os.Exit(0)
	}()
	if err := runScript(ctx, script, instanceName); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultCreateInstanceTimeout,
			}, nil
		}
		return nil, internalTestError{err, err.Error()}
	}

	// Check the test instance.
	printBanner(ctx, "Checking test instance")
	if err := checkTestInstance(ctx, root, instanceName); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultCheckInstanceTimeout,
			}, nil
		}
		return nil, internalTestError{err, err.Error()}
	}

	return &test.Result{Status: test.Passed}, nil
}

func cleanupTestInstances(ctx *tool.Context) error {
	printBanner(ctx, "Cleaning up test instances")

	// List all test instances.
	instances, err := listInstances(ctx, testInstancePrefix+".*")
	if err != nil {
		return err
	}

	// Delete them.
	for _, instance := range instances {
		if err := deleteInstance(ctx, instance.Name, instance.Zone); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v", err)
		}
	}
	return nil
}

func listInstances(ctx *tool.Context, instanceRegEx string) ([]instance, error) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	args := []string{
		"-q",
		"compute",
		"instances",
		"list",
		"--project",
		testInstanceProject,
		fmt.Sprintf("--regexp=%s", instanceRegEx),
		"--format=json",
	}
	if err := ctx.Run().CommandWithOpts(opts, "gcloud", args...); err != nil {
		return nil, err
	}
	instances := []instance{}
	if err := json.Unmarshal(out.Bytes(), &instances); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v", err)
	}
	return instances, nil
}

func deleteInstance(ctx *tool.Context, instanceName, instanceZone string) error {
	args := []string{
		"-q",
		"compute",
		"instances",
		"delete",
		"--project",
		testInstanceProject,
		"--zone",
		instanceZone,
		instanceName,
	}
	if err := ctx.Run().Command("gcloud", args...); err != nil {
		return err
	}
	return nil
}

func runScript(ctx *tool.Context, script, instanceName string) error {
	// Build all binaries.
	args := []string{"go", "install", "v.io/..."}
	if err := ctx.Run().Command("v23", args...); err != nil {
		return err
	}

	// Run script.
	args = []string{instanceName}
	if err := ctx.Run().TimedCommand(defaultCreateInstanceTimeout, script, args...); err != nil {
		return err
	}

	return nil
}

func checkTestInstance(ctx *tool.Context, root, instanceName string) error {
	instances, err := listInstances(ctx, instanceName)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return fmt.Errorf("no matching instance for %q", instanceName)
	}
	externalIP := instances[0].NetworkInterfaces[0].AccessConfigs[0].NatIP
	suites := testAllProdServices(ctx, root, "", fmt.Sprintf("/%s:8101", externalIP))
	allPassed := true
	for _, suite := range suites {
		allPassed = allPassed && (suite.Failures == 0)
	}
	if !allPassed {
		return fmt.Errorf("some checks failed")
	}
	return nil
}

func printBanner(ctx *tool.Context, msg string) {
	fmt.Fprintf(ctx.Stdout(), "##### %s #####\n", msg)
}
