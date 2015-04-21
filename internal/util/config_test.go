// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

var (
	goWorkspaces = []string{"test-go-workspace"}
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
	vdlWorkspaces            = []string{"test-vdl-workspace"}
	apiCheckRequiredProjects = []string{
		"projectA",
		"projectB",
	}
)

func testConfigAPI(t *testing.T, c *Config) {
	if got, want := c.GoWorkspaces(), goWorkspaces; !reflect.DeepEqual(got, want) {
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
	got, want := keys(c.ApiCheckRequiredProjects()), apiCheckRequiredProjects
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected results: got %v, want %v", got, want)
	}
}

func TestConfig(t *testing.T) {
	config := NewConfig(
		GoWorkspacesOpt(goWorkspaces),
		ProjectTestsOpt(projectTests),
		SnapshotLabelTestsOpt(snapshotLabelTests),
		TestDependenciesOpt(testDependencies),
		TestGroupsOpt(testGroups),
		TestPartsOpt(testParts),
		VDLWorkspacesOpt(vdlWorkspaces),
		ApiCheckRequiredProjectsOpt(apiCheckRequiredProjects),
	)

	testConfigAPI(t, config)
}

func TestConfigMarshal(t *testing.T) {
	config := NewConfig(
		GoWorkspacesOpt(goWorkspaces),
		ProjectTestsOpt(projectTests),
		SnapshotLabelTestsOpt(snapshotLabelTests),
		TestDependenciesOpt(testDependencies),
		TestGroupsOpt(testGroups),
		TestPartsOpt(testParts),
		VDLWorkspacesOpt(vdlWorkspaces),
		ApiCheckRequiredProjectsOpt(apiCheckRequiredProjects),
	)

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marhsall(%v) failed: %v", config, err)
	}
	var config2 Config
	if err := json.Unmarshal(data, &config2); err != nil {
		t.Fatalf("Unmarshall(%v) failed: %v", string(data), err)
	}

	testConfigAPI(t, &config2)
}
