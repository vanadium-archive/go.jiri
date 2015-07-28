// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"os"
	"reflect"
	"testing"

	"v.io/x/devtools/internal/tool"
)

var (
	apiCheckProjects = map[string]struct{}{
		"projectA": struct{}{},
		"projectB": struct{}{},
	}
	copyrightCheckProjects = map[string]struct{}{
		"projectC": struct{}{},
		"projectD": struct{}{},
	}
	goWorkspaces      = []string{"test-go-workspace"}
	jenkinsMatrixJobs = map[string]JenkinsMatrixJobInfo{
		"test-job-A": JenkinsMatrixJobInfo{
			HasArch:  false,
			HasOS:    true,
			HasParts: true,
			ShowOS:   false,
			Name:     "test-job-A",
		},
		"test-job-B": JenkinsMatrixJobInfo{
			HasArch:  true,
			HasOS:    false,
			HasParts: false,
			ShowOS:   false,
			Name:     "test-job-B",
		},
	}
	projectTests = map[string][]string{
		"test-project":  []string{"test-test-A", "test-test-group"},
		"test-project2": []string{"test-test-D"},
	}
	snapshotLabelTests = map[string][]string{
		"test-snapshot-label": []string{"test-test-A", "test-test-group"},
	}
	testDependencies = map[string][]string{
		"test-test-A": []string{"test-test-B"},
		"test-test-B": []string{"test-test-C"},
	}
	testGroups = map[string][]string{
		"test-test-group": []string{"test-test-B", "test-test-C"},
	}
	testParts = map[string][]string{
		"test-test-A": []string{"p1", "p2"},
	}
	vdlWorkspaces = []string{"test-vdl-workspace"}
)

func testConfigAPI(t *testing.T, c *Config) {
	if got, want := c.APICheckProjects(), apiCheckProjects; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected results: got %v, want %v", got, want)
	}
	if got, want := c.CopyrightCheckProjects(), copyrightCheckProjects; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected results: got %v, want %v", got, want)
	}
	if got, want := c.GoWorkspaces(), goWorkspaces; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.GroupTests([]string{"test-test-group"}), []string{"test-test-B", "test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.JenkinsMatrixJobs(), jenkinsMatrixJobs; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.Projects(), []string{"test-project", "test-project2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.ProjectTests([]string{"test-project"}), []string{"test-test-A", "test-test-B", "test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.ProjectTests([]string{"test-project", "test-project2"}), []string{"test-test-A", "test-test-B", "test-test-C", "test-test-D"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.SnapshotLabels(), []string{"test-snapshot-label"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.SnapshotLabelTests("test-snapshot-label"), []string{"test-test-A", "test-test-B", "test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.TestDependencies("test-test-A"), []string{"test-test-B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.TestDependencies("test-test-B"), []string{"test-test-C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.TestParts("test-test-A"), []string{"p1", "p2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
	if got, want := c.VDLWorkspaces(), vdlWorkspaces; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected result: got %v, want %v", got, want)
	}
}

func TestConfigAPI(t *testing.T) {
	config := NewConfig(
		APICheckProjectsOpt(apiCheckProjects),
		CopyrightCheckProjectsOpt(copyrightCheckProjects),
		GoWorkspacesOpt(goWorkspaces),
		JenkinsMatrixJobsOpt(jenkinsMatrixJobs),
		ProjectTestsOpt(projectTests),
		SnapshotLabelTestsOpt(snapshotLabelTests),
		TestDependenciesOpt(testDependencies),
		TestGroupsOpt(testGroups),
		TestPartsOpt(testParts),
		VDLWorkspacesOpt(vdlWorkspaces),
	)

	testConfigAPI(t, config)
}

func TestConfigSerialization(t *testing.T) {
	ctx := tool.NewDefaultContext()

	root, err := NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()
	oldRoot, err := V23Root()
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)

	config := NewConfig(
		APICheckProjectsOpt(apiCheckProjects),
		CopyrightCheckProjectsOpt(copyrightCheckProjects),
		GoWorkspacesOpt(goWorkspaces),
		JenkinsMatrixJobsOpt(jenkinsMatrixJobs),
		ProjectTestsOpt(projectTests),
		SnapshotLabelTestsOpt(snapshotLabelTests),
		TestDependenciesOpt(testDependencies),
		TestGroupsOpt(testGroups),
		TestPartsOpt(testParts),
		VDLWorkspacesOpt(vdlWorkspaces),
	)

	root.WriteLocalToolsConfig(ctx, config)
	config2, err := root.ReadLocalToolsConfig(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	testConfigAPI(t, config2)
}
