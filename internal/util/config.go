// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"encoding/json"
	"sort"
)

// Config holds configuration common to vanadium tools.
type Config struct {
	// goWorkspaces identifies V23_ROOT subdirectories that contain a
	// Go workspace.
	goWorkspaces []string
	// projectTests maps vanadium projects to sets of tests that should be
	// executed to test changes in the given project.
	projectTests map[string][]string
	// snapshotLabelTests maps snapshot labels to sets of tests that
	// determine whether a snapshot for the given label can be created.
	snapshotLabelTests map[string][]string
	// testDependencies maps tests to sets of tests that the given test
	// depends on.
	testDependencies map[string][]string
	// testGroups maps test group labels to sets of tests that the label
	// identifies.
	testGroups map[string][]string
	// testParts maps test names to lists of strings that identify
	// different parts of a test. If a list L has n elements, then the
	// corresponding test has n+1 parts: the first n parts are identified
	// by L[0] to L[n-1]. The last part is whatever is left.
	testParts map[string][]string
	// vdlWorkspaces identifies V23_ROOT subdirectories that contain
	// a VDL workspace.
	vdlWorkspaces []string
	// apiCheckRequiredProjects identifies the set of project names for
	// which an API check is required.
	apiCheckRequiredProjects map[string]bool
}

// ConfigOpt is an interface for Config factory options.
type ConfigOpt interface {
	configOpt()
}

// GoWorkspaceOpt is the type that can be used to pass the Config
// factory a Go workspace option.
type GoWorkspaceOpt []string

func (GoWorkspaceOpt) configOpt() {}

// ProjectTestsOpt is the type that can be used to pass the Config
// factory a project tests option.
type ProjectTestsOpt map[string][]string

func (ProjectTestsOpt) configOpt() {}

// SnapshotLabelTestsOpt is the type that can be used to pass the
// Config factory a snapshot label tests option.
type SnapshotLabelTestsOpt map[string][]string

func (SnapshotLabelTestsOpt) configOpt() {}

// TestDependenciesOpt is the type that can be used to pass the Config
// factory a test dependencies option.
type TestDependenciesOpt map[string][]string

func (TestDependenciesOpt) configOpt() {}

// TestGroupsOpt is the type that can be used to pass the Config
// factory a test groups option.
type TestGroupsOpt map[string][]string

func (TestGroupsOpt) configOpt() {}

// TestPartsOpt is the type that can be used to pass the Config
// factory a test parts option.
type TestPartsOpt map[string][]string

func (TestPartsOpt) configOpt() {}

// VDLWorkspacesOpt is the type that can be used to pass the Config
// factory a VDL workspace option.
type VDLWorkspacesOpt []string

func (VDLWorkspacesOpt) configOpt() {}

type ApiCheckRequiredProjectsOpt []string

func (ApiCheckRequiredProjectsOpt) configOpt() {}

func listToSet(keys []string) map[string]bool {
	result := make(map[string]bool)
	for _, key := range keys {
		result[key] = true
	}
	return result
}

func keys(set map[string]bool) []string {
	var result []string
	for key, _ := range set {
		result = append(result, key)
	}
	return result
}

// NewConfig is the Config factory.
func NewConfig(opts ...ConfigOpt) *Config {
	var c Config
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case GoWorkspaceOpt:
			c.goWorkspaces = []string(typedOpt)
		case ProjectTestsOpt:
			c.projectTests = map[string][]string(typedOpt)
		case SnapshotLabelTestsOpt:
			c.snapshotLabelTests = map[string][]string(typedOpt)
		case TestDependenciesOpt:
			c.testDependencies = map[string][]string(typedOpt)
		case TestGroupsOpt:
			c.testGroups = map[string][]string(typedOpt)
		case TestPartsOpt:
			c.testParts = map[string][]string(typedOpt)
		case VDLWorkspacesOpt:
			c.vdlWorkspaces = []string(typedOpt)
		case ApiCheckRequiredProjectsOpt:
			c.apiCheckRequiredProjects = listToSet([]string(typedOpt))
		}
	}
	return &c
}

// GoWorkspaces returns the Go workspaces included in the config.
func (c Config) GoWorkspaces() []string {
	return c.goWorkspaces
}

// Projects returns a list of projects included in the config.
func (c Config) Projects() (projects []string) {
	for project, _ := range c.projectTests {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return
}

// ProjectTests returns a list of Jenkins tests associated with the
// given projects by the config.
func (c Config) ProjectTests(projects []string) []string {
	testSet := map[string]struct{}{}
	testGroups := c.testGroups

	for _, project := range projects {
		for _, test := range c.projectTests[project] {
			if testGroup, ok := testGroups[test]; ok {
				for _, test := range testGroup {
					testSet[test] = struct{}{}
				}
			} else {
				testSet[test] = struct{}{}
			}
		}
	}
	sortedTests := []string{}
	for test := range testSet {
		sortedTests = append(sortedTests, test)
	}
	sort.Strings(sortedTests)
	return sortedTests
}

// SnapshotLabels returns a list of snapshot labels included in the
// config.
func (c Config) SnapshotLabels() (labels []string) {
	for label, _ := range c.snapshotLabelTests {
		labels = append(labels, label)
	}
	return
}

// SnapshotLabelTests returns a list of tests for the given label.
func (c Config) SnapshotLabelTests(label string) (tests []string) {
	testGroups := c.testGroups
	for _, test := range c.snapshotLabelTests[label] {
		if testGroup, ok := testGroups[test]; ok {
			for _, test := range testGroup {
				tests = append(tests, test)
			}
		} else {
			tests = append(tests, test)
		}
	}
	return
}

// TestDependencies returns a list of dependencies for the given test.
func (c Config) TestDependencies(test string) []string {
	return c.testDependencies[test]
}

// TestParts returns a list of strings that identify different test parts.
func (c Config) TestParts(test string) []string {
	return c.testParts[test]
}

// VDLWorkspaces returns the VDL workspaces included in the config.
func (c Config) VDLWorkspaces() []string {
	return c.vdlWorkspaces
}

// ApiCheckRequiredProjects returns the set of project names for which an API
// check is required.
func (c Config) ApiCheckRequiredProjects() map[string]bool {
	return c.apiCheckRequiredProjects
}

type config struct {
	GoWorkspaces             []string            `json:"go-workspaces-new"`
	ProjectTests             map[string][]string `json:"project-tests"`
	SnapshotLabelTests       map[string][]string `json:"snapshot-label-tests"`
	TestDependencies         map[string][]string `json:"test-dependencies"`
	TestGroups               map[string][]string `json:"test-groups"`
	TestParts                map[string][]string `json:"test-parts"`
	VDLWorkspaces            []string            `json:"vdl-workspaces-new"`
	ApiCheckRequiredProjects []string            `json:"api-check-required-projects"`
}

var _ json.Marshaler = (*Config)(nil)

func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(config{
		GoWorkspaces:             c.goWorkspaces,
		ProjectTests:             c.projectTests,
		SnapshotLabelTests:       c.snapshotLabelTests,
		TestDependencies:         c.testDependencies,
		TestGroups:               c.testGroups,
		TestParts:                c.testParts,
		VDLWorkspaces:            c.vdlWorkspaces,
		ApiCheckRequiredProjects: keys(c.apiCheckRequiredProjects),
	})
}

var _ json.Unmarshaler = (*Config)(nil)

func (c *Config) UnmarshalJSON(data []byte) error {
	var conf config
	if err := json.Unmarshal(data, &conf); err != nil {
		return err
	}
	c.goWorkspaces = conf.GoWorkspaces
	c.projectTests = conf.ProjectTests
	c.snapshotLabelTests = conf.SnapshotLabelTests
	c.testDependencies = conf.TestDependencies
	c.testGroups = conf.TestGroups
	c.testParts = conf.TestParts
	c.vdlWorkspaces = conf.VDLWorkspaces
	c.apiCheckRequiredProjects = listToSet(conf.ApiCheckRequiredProjects)
	return nil
}
