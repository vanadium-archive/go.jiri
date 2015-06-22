// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/set"
)

// Config holds configuration common to vanadium tools.
type Config struct {
	// apiCheckProjects identifies the set of project names for which
	// the API check is required.
	apiCheckProjects map[string]struct{}
	// copyrightCheckProjects identifies the set of project names for
	// which the copyright check is required.
	copyrightCheckProjects map[string]struct{}
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
}

// ConfigOpt is an interface for Config factory options.
type ConfigOpt interface {
	configOpt()
}

// APICheckProjectsOpt is the type that can be used to pass the Config
// factory a API check projects option.
type APICheckProjectsOpt map[string]struct{}

func (APICheckProjectsOpt) configOpt() {}

// CopyrightCheckProjectsOpt is the type that can be used to pass the
// Config factory a copyright check projects option.
type CopyrightCheckProjectsOpt map[string]struct{}

func (CopyrightCheckProjectsOpt) configOpt() {}

// GoWorkspacesOpt is the type that can be used to pass the Config
// factory a Go workspace option.
type GoWorkspacesOpt []string

func (GoWorkspacesOpt) configOpt() {}

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

// NewConfig is the Config factory.
func NewConfig(opts ...ConfigOpt) *Config {
	var c Config
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case APICheckProjectsOpt:
			c.apiCheckProjects = map[string]struct{}(typedOpt)
		case CopyrightCheckProjectsOpt:
			c.copyrightCheckProjects = map[string]struct{}(typedOpt)
		case GoWorkspacesOpt:
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
		}
	}
	return &c
}

// APICheckProjects returns the set of project names for which the API
// check is required.
func (c Config) APICheckProjects() map[string]struct{} {
	return c.apiCheckProjects
}

// CopyrightCheckProjects returns the set of project names for which
// the copyright check is required.
func (c Config) CopyrightCheckProjects() map[string]struct{} {
	return c.copyrightCheckProjects
}

// GoWorkspaces returns the Go workspaces included in the config.
func (c Config) GoWorkspaces() []string {
	return c.goWorkspaces
}

// Projects returns a list of projects included in the config.
func (c Config) Projects() []string {
	var projects []string
	for project, _ := range c.projectTests {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return projects
}

// ProjectTests returns a list of Jenkins tests associated with the
// given projects by the config.
func (c Config) ProjectTests(projects []string) []string {
	testSet := map[string]struct{}{}
	testGroups := c.testGroups
	for _, project := range projects {
		for _, test := range c.projectTests[project] {
			if testGroup, ok := testGroups[test]; ok {
				set.String.Union(testSet, set.String.FromSlice(testGroup))
			} else {
				testSet[test] = struct{}{}
			}
		}
	}
	tests := set.String.ToSlice(testSet)
	sort.Strings(tests)
	return tests
}

// SnapshotLabels returns a list of snapshot labels included in the
// config.
func (c Config) SnapshotLabels() []string {
	var labels []string
	for label, _ := range c.snapshotLabelTests {
		labels = append(labels, label)
	}
	return labels
}

// SnapshotLabelTests returns a list of tests for the given label.
func (c Config) SnapshotLabelTests(label string) []string {
	testSet := map[string]struct{}{}
	testGroups := c.testGroups
	for _, test := range c.snapshotLabelTests[label] {
		if testGroup, ok := testGroups[test]; ok {
			set.String.Union(testSet, set.String.FromSlice(testGroup))
		} else {
			testSet[test] = struct{}{}
		}
	}
	tests := set.String.ToSlice(testSet)
	sort.Strings(tests)
	return tests
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

type configSchema struct {
	APICheckProjects       []string               `xml:"apiCheckProjects>project"`
	CopyrightCheckProjects []string               `xml:"copyrightCheckProjects>project"`
	GoWorkspaces           []string               `xml:"goWorkspaces>workspace"`
	ProjectTests           testGroupSchemas       `xml:"projectTests>project"`
	SnapshotLabelTests     testGroupSchemas       `xml:"snapshotLabelTests>snapshot"`
	TestDependencies       dependencyGroupSchemas `xml:"testDependencies>test"`
	TestGroups             testGroupSchemas       `xml:"testGroups>group"`
	TestParts              partGroupSchemas       `xml:"testParts>test"`
	VDLWorkspaces          []string               `xml:"vdlWorkspaces>workspace"`
	XMLName                xml.Name               `xml:"config"`
}

type dependencyGroupSchema struct {
	Name         string   `xml:"name,attr"`
	Dependencies []string `xml:"dependency"`
}

type dependencyGroupSchemas []dependencyGroupSchema

func (d dependencyGroupSchemas) Len() int           { return len(d) }
func (d dependencyGroupSchemas) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d dependencyGroupSchemas) Less(i, j int) bool { return d[i].Name < d[j].Name }

type partGroupSchema struct {
	Name  string   `xml:"name,attr"`
	Parts []string `xml:"part"`
}

type partGroupSchemas []partGroupSchema

func (p partGroupSchemas) Len() int           { return len(p) }
func (p partGroupSchemas) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p partGroupSchemas) Less(i, j int) bool { return p[i].Name < p[j].Name }

type testGroupSchema struct {
	Name  string   `xml:"name,attr"`
	Tests []string `xml:"test"`
}

type testGroupSchemas []testGroupSchema

func (p testGroupSchemas) Len() int           { return len(p) }
func (p testGroupSchemas) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p testGroupSchemas) Less(i, j int) bool { return p[i].Name < p[j].Name }

// LoadConfig returns the configuration stored in the tools
// configuration file.
func LoadConfig(ctx *tool.Context) (*Config, error) {
	configPath, err := ConfigFilePath(ctx)
	if err != nil {
		return nil, err
	}
	return loadConfig(ctx, configPath)
}

func loadConfig(ctx *tool.Context, path string) (*Config, error) {
	configBytes, err := ctx.Run().ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data configSchema
	if err := xml.Unmarshal(configBytes, &data); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(configBytes), err)
	}
	config := &Config{
		apiCheckProjects:       map[string]struct{}{},
		copyrightCheckProjects: map[string]struct{}{},
		goWorkspaces:           []string{},
		projectTests:           map[string][]string{},
		snapshotLabelTests:     map[string][]string{},
		testDependencies:       map[string][]string{},
		testGroups:             map[string][]string{},
		testParts:              map[string][]string{},
		vdlWorkspaces:          []string{},
	}
	config.apiCheckProjects = set.String.FromSlice(data.APICheckProjects)
	config.copyrightCheckProjects = set.String.FromSlice(data.CopyrightCheckProjects)
	for _, workspace := range data.GoWorkspaces {
		config.goWorkspaces = append(config.goWorkspaces, workspace)
	}
	sort.Strings(config.goWorkspaces)
	for _, project := range data.ProjectTests {
		config.projectTests[project.Name] = project.Tests
	}
	for _, snapshot := range data.SnapshotLabelTests {
		config.snapshotLabelTests[snapshot.Name] = snapshot.Tests
	}
	for _, test := range data.TestDependencies {
		config.testDependencies[test.Name] = test.Dependencies
	}
	for _, group := range data.TestGroups {
		config.testGroups[group.Name] = group.Tests
	}
	for _, test := range data.TestParts {
		config.testParts[test.Name] = test.Parts
	}
	for _, workspace := range data.VDLWorkspaces {
		config.vdlWorkspaces = append(config.vdlWorkspaces, workspace)
	}
	sort.Strings(config.vdlWorkspaces)
	return config, nil
}

// SaveConfig writes the given configuration to the tools
// configuration file.
func SaveConfig(ctx *tool.Context, config *Config) error {
	configPath, err := ConfigFilePath(ctx)
	if err != nil {
		return err
	}
	return saveConfig(ctx, config, configPath)
}

func saveConfig(ctx *tool.Context, config *Config, path string) error {
	var data configSchema
	data.APICheckProjects = set.String.ToSlice(config.apiCheckProjects)
	sort.Strings(data.APICheckProjects)
	data.CopyrightCheckProjects = set.String.ToSlice(config.copyrightCheckProjects)
	sort.Strings(data.CopyrightCheckProjects)
	for _, workspace := range config.goWorkspaces {
		data.GoWorkspaces = append(data.GoWorkspaces, workspace)
	}
	sort.Strings(data.GoWorkspaces)
	for name, tests := range config.projectTests {
		data.ProjectTests = append(data.ProjectTests, testGroupSchema{
			Name:  name,
			Tests: tests,
		})
	}
	sort.Sort(data.ProjectTests)
	for name, tests := range config.snapshotLabelTests {
		data.SnapshotLabelTests = append(data.SnapshotLabelTests, testGroupSchema{
			Name:  name,
			Tests: tests,
		})
	}
	sort.Sort(data.SnapshotLabelTests)
	for name, dependencies := range config.testDependencies {
		data.TestDependencies = append(data.TestDependencies, dependencyGroupSchema{
			Name:         name,
			Dependencies: dependencies,
		})
	}
	sort.Sort(data.TestDependencies)
	for name, tests := range config.testGroups {
		data.TestGroups = append(data.TestGroups, testGroupSchema{
			Name:  name,
			Tests: tests,
		})
	}
	sort.Sort(data.TestGroups)
	for name, parts := range config.testParts {
		data.TestParts = append(data.TestParts, partGroupSchema{
			Name:  name,
			Parts: parts,
		})
	}
	sort.Sort(data.TestParts)
	for _, workspace := range config.vdlWorkspaces {
		data.VDLWorkspaces = append(data.VDLWorkspaces, workspace)
	}
	sort.Strings(data.VDLWorkspaces)
	bytes, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", data, err)
	}
	if err := ctx.Run().WriteFile(path, bytes, os.FileMode(0644)); err != nil {
		return err
	}
	return nil
}
