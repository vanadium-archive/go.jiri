// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/googlesource"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

var JiriProject = "release.go.jiri"
var JiriName = "jiri"
var JiriPackage = "v.io/jiri"

// CL represents a changelist.
type CL struct {
	// Author identifies the author of the changelist.
	Author string
	// Email identifies the author's email.
	Email string
	// Description holds the description of the changelist.
	Description string
}

// Manifest represents a setting used for updating the universe.
type Manifest struct {
	Hooks    []Hook    `xml:"hooks>hook"`
	Hosts    []Host    `xml:"hosts>host"`
	Imports  []Import  `xml:"imports>import"`
	Label    string    `xml:"label,attr"`
	Projects []Project `xml:"projects>project"`
	Tools    []Tool    `xml:"tools>tool"`
	XMLName  struct{}  `xml:"manifest"`
}

// Hooks maps hook names to their detailed description.
type Hooks map[string]Hook

// Hook represents a post-update project hook.
type Hook struct {
	// Exclude is flag used to exclude previously included hooks.
	Exclude bool `xml:"exclude,attr"`
	// Name is the hook name.
	Name string `xml:"name,attr"`
	// Project is the name of the project the hook is associated with.
	Project string `xml:"project,attr"`
	// Path is the path of the hook relative to its project's root.
	Path string `xml:"path,attr"`
	// Interpreter is an optional program used to interpret the hook (i.e. python). Unlike Path,
	// Interpreter is relative to the environment's PATH and not the project's root.
	Interpreter string `xml:"interpreter,attr"`
	// Arguments for the hook.
	Args []HookArg `xml:"arg"`
}

type HookArg struct {
	Arg string `xml:",chardata"`
}

// Hosts map host name to their detailed description.
type Hosts map[string]Host

// Host represents the locations of git and gerrit repository hosts.
type Host struct {
	// Name is the host name.
	Name string `xml:"name,attr"`
	// Location is the url of the host.
	Location string `xml:"location,attr"`
	// Git hooks to apply to repos from this host.
	GitHooks []GitHook `xml:"githooks>githook"`
}

// GitHook represents the name and source of git hooks.
type GitHook struct {
	// The hook name, as required by git (e.g. commit-msg, pre-rebase, etc.)
	Name string `xml:"name,attr"`
	// The filename of the hook implementation.  When editing the manifest,
	// specify this path as relative to the manifest dir.  In loadManifest,
	// this gets resolved to the absolute path.
	Path string `xml:"path,attr"`
}

// Imports maps manifest import names to their detailed description.
type Imports map[string]Import

// Import represnts a manifest import.
type Import struct {
	// Name is the name under which the manifest can be found the
	// manifest repository.
	Name string `xml:"name,attr"`
}

// Projects maps project names to their detailed description.
type Projects map[string]Project

// Project represents a jiri project.
type Project struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Name is the project name.
	Name string `xml:"name,attr"`
	// Path is the path used to store the project locally. Project
	// manifest uses paths that are relative to the $JIRI_ROOT
	// environment variable. When a manifest is parsed (e.g. in
	// RemoteProjects), the program logic converts the relative
	// paths to an absolute paths, using the current value of the
	// $JIRI_ROOT environment variable as a prefix.
	Path string `xml:"path,attr"`
	// Protocol is the version control protocol used by the
	// project. If not set, "git" is used as the default.
	Protocol string `xml:"protocol,attr"`
	// Remote is the project remote.
	Remote string `xml:"remote,attr"`
	// Revision is the revision the project should be advanced to
	// during "jiri update". If not set, "HEAD" is used as the
	// default.
	Revision string `xml:"revision,attr"`
}

// Tools maps jiri tool names, to their detailed description.
type Tools map[string]Tool

// Tool represents a jiri tool.
type Tool struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Data is a relative path to a directory for storing tool data
	// (e.g. tool configuration files). The purpose of this field is to
	// decouple the configuration of the data directory from the tool
	// itself so that the location of the data directory can change
	// without the need to change the tool.
	Data string `xml:"data,attr"`
	// Name is the name of the tool binary.
	Name string `xml:"name,attr"`
	// Package is the package path of the tool.
	Package string `xml:"package,attr"`
	// Project identifies the project that contains the tool. If not
	// set, "https://vanadium.googlesource.com/<JiriProject>" is
	// used as the default.
	Project string `xml:"project,attr"`
}

type UnsupportedProtocolErr string

func (e UnsupportedProtocolErr) Error() string {
	return fmt.Sprintf("unsupported protocol %v", e)
}

// Update represents an update of projects as a map from
// project names to a collections of commits.
type Update map[string][]CL

var devtoolsBinDir = filepath.Join("devtools", "bin")

// CreateSnapshot creates a manifest that encodes the current state of
// master branches of all projects and writes this snapshot out to the
// given file.
func CreateSnapshot(ctx *tool.Context, path string) error {
	// Create an in-memory representation of the current projects.
	manifest, err := snapshotLocalProjects(ctx)
	if err != nil {
		return err
	}

	// Add all hosts, tools, and hooks from the current manifest to the snapshot
	// manifest.
	hosts, _, tools, hooks, err := readManifest(ctx, true)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		manifest.Tools = append(manifest.Tools, tool)
	}
	for _, host := range hosts {
		manifest.Hosts = append(manifest.Hosts, host)
	}
	for _, hook := range hooks {
		manifest.Hooks = append(manifest.Hooks, hook)
	}

	perm := os.FileMode(0755)
	if err := ctx.Run().MkdirAll(filepath.Dir(path), perm); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", manifest, err)
	}
	perm = os.FileMode(0644)
	if err := ctx.Run().WriteFile(path, data, perm); err != nil {
		return err
	}
	return nil
}

const currentManifestFileName = ".current_manifest"

// CurrentManifest returns a manifest that identifies the result of
// the most recent "jiri update" invocation.
func CurrentManifest(ctx *tool.Context) (*Manifest, error) {
	root, err := JiriRoot()
	if err != nil {
		return nil, err
	}
	currentManifestFile := filepath.Join(root, currentManifestFileName)
	bytes, err := ctx.Run().ReadFile(currentManifestFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(ctx.Stderr(), `WARNING: Could not find %s.
The contents of this file are stored as metadata in binaries the jiri
tool builds. To fix this problem, please run "jiri update".
`, currentManifestFile)
			return &Manifest{}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := xml.Unmarshal(bytes, &m); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	return &m, nil
}

// writeCurrentManifest writes the given manifest to a file that
// stores the result of the most recent "jiri update" invocation.
func writeCurrentManifest(ctx *tool.Context, manifest *Manifest) error {
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	bytes, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", manifest, err)
	}
	if err := ctx.Run().WriteFile(filepath.Join(root, currentManifestFileName), bytes, os.FileMode(0644)); err != nil {
		return err
	}
	return nil
}

// CurrentProjectName gets the name of the current project from the
// current directory by reading the jiri project metadata located in a
// directory at the root of the current repository.
func CurrentProjectName(ctx *tool.Context) (string, error) {
	topLevel, err := ctx.Git().TopLevel()
	if err != nil {
		return "", nil
	}
	metadataDir := filepath.Join(topLevel, metadataDirName)
	if _, err := ctx.Run().Stat(metadataDir); err == nil {
		metadataFile := filepath.Join(metadataDir, metadataFileName)
		bytes, err := ctx.Run().ReadFile(metadataFile)
		if err != nil {
			return "", err
		}
		var project Project
		if err := xml.Unmarshal(bytes, &project); err != nil {
			return "", fmt.Errorf("Unmarshal() failed: %v", err)
		}
		return project.Name, nil
	}
	return "", nil
}

// LocalProjects scans the local filesystem to identify existing projects.
func LocalProjects(ctx *tool.Context) (Projects, error) {
	root, err := JiriRoot()
	if err != nil {
		return nil, err
	}
	projects := Projects{}
	if err := findLocalProjects(ctx, root, metadataDirName, projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// PollProjects returns the set of changelists that exist remotely but not
// locally. Changes are grouped by projects and contain author identification
// and a description of their content.
func PollProjects(ctx *tool.Context, projectSet map[string]struct{}) (_ Update, e error) {
	update := Update{}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	_, remoteProjects, _, _, err := readManifest(ctx, false)
	if err != nil {
		return nil, err
	}
	ops, err := computeOperations(localProjects, remoteProjects, false)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		name := op.Project().Name
		if len(projectSet) > 0 {
			if _, ok := projectSet[name]; !ok {
				continue
			}
		}
		cls := []CL{}
		if updateOp, ok := op.(updateOperation); ok {
			switch updateOp.project.Protocol {
			case "git":
				if err := ctx.Run().Chdir(updateOp.destination); err != nil {
					return nil, err
				}
				if err := ctx.Git().Fetch("origin", "master"); err != nil {
					return nil, err
				}
				commitsText, err := ctx.Git().Log("FETCH_HEAD", "master", "%an%n%ae%n%B")
				if err != nil {
					return nil, err
				}
				for _, commitText := range commitsText {
					if got, want := len(commitText), 3; got < want {
						return nil, fmt.Errorf("Unexpected length of %v: got %v, want at least %v", commitText, got, want)
					}
					cls = append(cls, CL{
						Author:      commitText[0],
						Email:       commitText[1],
						Description: strings.Join(commitText[2:], "\n"),
					})
				}
			default:
				return nil, UnsupportedProtocolErr(updateOp.project.Protocol)
			}
		}
		update[name] = cls
	}
	return update, nil
}

// ReadManifest retrieves and parses the manifest that determines what
// projects and tools are part of the jiri universe.
func ReadManifest(ctx *tool.Context) (Projects, Tools, error) {
	_, p, t, _, e := readManifest(ctx, false)
	return p, t, e
}

// readManifest implements the ReadManifest logic and provides an
// optional flag that can be used to fetch the latest manifest updates
// from the manifest repository.
func readManifest(ctx *tool.Context, update bool) (Hosts, Projects, Tools, Hooks, error) {
	if update {
		root, err := JiriRoot()
		if err != nil {
			return nil, nil, nil, nil, err
		}
		project := Project{
			Path:     filepath.Join(root, ".manifest"),
			Protocol: "git",
			Revision: "HEAD",
		}
		if err := pullProject(ctx, project); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	path, err := ResolveManifestPath(ctx.Manifest())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	hosts, projects, tools, hooks, stack := Hosts{}, Projects{}, Tools{}, Hooks{}, map[string]struct{}{}
	if err := loadManifest(ctx, path, hosts, projects, tools, hooks, stack); err != nil {
		return nil, nil, nil, nil, err
	}
	return hosts, projects, tools, hooks, nil
}

// UpdateUniverse updates all local projects and tools to match the
// remote counterparts identified by the given manifest. Optionally,
// the 'gc' flag can be used to indicate that local projects that no
// longer exist remotely should be removed.
func UpdateUniverse(ctx *tool.Context, gc bool) (e error) {
	_, remoteProjects, remoteTools, remoteHooks, err := readManifest(ctx, true)
	if err != nil {
		return err
	}
	// 1. Update all local projects to match their remote counterparts.
	if err := updateProjects(ctx, remoteProjects, gc); err != nil {
		return err
	}
	// 2. Build all tools in a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "tmp-jiri-tools-build")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
	if err := buildTools(ctx, remoteTools, tmpDir); err != nil {
		return err
	}
	// 3. Install the tools into $JIRI_ROOT/devtools/bin.
	if err := InstallTools(ctx, tmpDir); err != nil {
		return err
	}
	// 4. Run all specified hooks
	return runHooks(ctx, remoteHooks)
}

// ApplyToLocalMaster applies an operation expressed as the given
// function to the local master branch of the given project.
func ApplyToLocalMaster(ctx *tool.Context, project Project, fn func() error) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(project.Path); err != nil {
		return err
	}
	switch project.Protocol {
	case "git":
		branch, err := ctx.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		stashed, err := ctx.Git().Stash()
		if err != nil {
			return err
		}
		if stashed {
			defer collect.Error(func() error { return ctx.Git().StashPop() }, &e)
		}
		if err := ctx.Git().CheckoutBranch("master"); err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Git().CheckoutBranch(branch) }, &e)
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
	return fn()
}

// BuildTool builds the given tool specified by its name and package, sets its
// version by counting the number of commits in current version-controlled
// directory, and places the resulting binary into the given directory.
func BuildTool(ctx *tool.Context, outputDir, name, pkg string, toolsProject Project) error {
	// Change to tools project's local dir.
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer ctx.Run().Chdir(wd)
	if err := ctx.Run().Chdir(toolsProject.Path); err != nil {
		return err
	}
	// Identify the Go workspace the tool is in. To this end we use a
	// heuristic that identifies the maximal suffix of the project path
	// that corresponds to a prefix of the package name.
	workspace, path := "", toolsProject.Path
	for i := 0; i < len(path); i++ {
		if path[i] == filepath.Separator {
			if strings.HasPrefix("src/"+pkg, filepath.ToSlash(path[i+1:])) {
				workspace = path[:i]
				break
			}
		}
	}
	if workspace == "" {
		return fmt.Errorf("could not identify go workspace for %v", pkg)
	}
	workspaces := []string{workspace}
	if envGoPath := os.Getenv("GOPATH"); envGoPath != "" {
		workspaces = append(workspaces, strings.Split(envGoPath, string(filepath.ListSeparator))...)
	}
	output := filepath.Join(outputDir, name)
	count := 0
	switch toolsProject.Protocol {
	case "git":
		gitCount, err := ctx.Git().CountCommits("HEAD", "")
		if err != nil {
			return err
		}
		count = gitCount
	default:
		return UnsupportedProtocolErr(toolsProject.Protocol)
	}
	ldflags := fmt.Sprintf("-X v.io/jiri/tool.Name %s -X v.io/jiri/tool.Version %d", name, count)
	args := []string{"build", "-ldflags", ldflags, "-o", output, pkg}
	var stderr bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Env = map[string]string{
		"GOPATH": strings.Join(workspaces, string(filepath.ListSeparator)),
	}
	opts.Stdout = ioutil.Discard
	opts.Stderr = &stderr
	if err := ctx.Run().CommandWithOpts(opts, "go", args...); err != nil {
		return fmt.Errorf("%v tool build failed\n%v", name, stderr.String())
	}
	return nil
}

// buildTools builds and installs all jiri tools using the version available in
// the local master branch of the tools repository. Notably, this function does
// not perform any version control operation on the master branch.
func buildTools(ctx *tool.Context, remoteTools Tools, outputDir string) error {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	failed := false
	names := []string{}
	for name, _ := range remoteTools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tool := remoteTools[name]
		// Skip tools with no package specified. Besides increasing
		// robustness, this step also allows us to create jiri root
		// fakes without having to provide an implementation for the "jiri"
		// tool, which every manifest needs to specify.
		if tool.Package == "" {
			continue
		}
		updateFn := func() error {
			project, ok := localProjects[tool.Project]
			if !ok {
				return fmt.Errorf("unknown project %v", tool.Project)
			}
			return ApplyToLocalMaster(ctx, project, func() error {
				return BuildTool(ctx, outputDir, tool.Name, tool.Package, project)
			})
		}
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		opts := runutil.Opts{Verbose: true}
		if err := ctx.Run().FunctionWithOpts(opts, updateFn, "build tool %q", tool.Name); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// CleanupProjects restores the given jiri projects back to their master
// branches and gets rid of all the local changes. If "cleanupBranches" is
// true, it will also delete all the non-master branches.
func CleanupProjects(ctx *tool.Context, projects Projects, cleanupBranches bool) (e error) {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(wd) }, &e)
	for _, project := range projects {
		localProjectDir := project.Path
		if err := ctx.Run().Chdir(localProjectDir); err != nil {
			return err
		}
		if err := resetLocalProject(ctx, cleanupBranches); err != nil {
			return err
		}
	}
	return nil
}

// resetLocalProject checks out the master branch, cleans up untracked files
// and uncommitted changes, and optionally deletes all the other branches.
func resetLocalProject(ctx *tool.Context, cleanupBranches bool) error {
	// Check out master and clean up changes.
	curBranchName, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	if curBranchName != "master" {
		if err := ctx.Git().CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	if err := ctx.Git().RemoveUntrackedFiles(); err != nil {
		return err
	}
	// Discard any uncommitted changes.
	if err := ctx.Git().Reset("origin/master"); err != nil {
		return err
	}

	// Delete all the other branches.
	// At this point we should be at the master branch.
	branches, _, err := ctx.Git().GetBranches()
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch == "master" {
			continue
		}
		if cleanupBranches {
			if err := ctx.Git().DeleteBranch(branch, gitutil.ForceOpt(true)); err != nil {
				return nil
			}
		}
	}

	return nil
}

// findLocalProjects implements LocalProjects.
func findLocalProjects(ctx *tool.Context, path, metadataDirName string, projects Projects) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(path); err != nil {
		return err
	}
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	metadataDir := filepath.Join(path, metadataDirName)
	if _, err := ctx.Run().Stat(metadataDir); err == nil {
		metadataFile := filepath.Join(metadataDir, metadataFileName)
		bytes, err := ctx.Run().ReadFile(metadataFile)
		if err != nil {
			return err
		}
		var project Project
		if err := xml.Unmarshal(bytes, &project); err != nil {
			return fmt.Errorf("Unmarshal() failed: %v\n%s", err, string(bytes))
		}
		if p, ok := projects[project.Name]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain the project %v", p.Path, project.Path, project.Name)
		}
		// Root relative paths in the $JIRI_ROOT directory.
		if !filepath.IsAbs(project.Path) {
			project.Path = filepath.Join(root, project.Path)
		}
		projects[project.Name] = project
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	fileInfos, err := ctx.Run().ReadDir(path)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() && !strings.HasPrefix(fileInfo.Name(), ".") {
			if err := findLocalProjects(ctx, filepath.Join(path, fileInfo.Name()), metadataDirName, projects); err != nil {
				return err
			}
		}
	}
	return nil
}

// InstallTools installs the tools from the given directory into
// $JIRI_ROOT/devtools/bin.
func InstallTools(ctx *tool.Context, dir string) error {
	if ctx.DryRun() {
		// In "dry run" mode, no binaries are built.
		return nil
	}
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", dir, err)
	}
	failed := false
	binDir := filepath.Join(root, devtoolsBinDir)
	for _, fi := range fis {
		installFn := func() error {
			src := filepath.Join(dir, fi.Name())
			dst := filepath.Join(binDir, fi.Name())
			if err := ctx.Run().Rename(src, dst); err != nil {
				return err
			}
			return nil
		}
		opts := runutil.Opts{Verbose: true}
		if err := ctx.Run().FunctionWithOpts(opts, installFn, "install tool %q", fi.Name()); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}

	// Rename ".v23_profiles" to ".jiri_profiles" iff ".jiri_profiles" doesn't exist"
	// TODO(nlacasse): Remove this code once the v23->jiri transition is
	// complete and everybody has had time to update.
	v23ProfilesFile := filepath.Join(root, ".v23_profiles")
	jiriProfilesFile := filepath.Join(root, ".jiri_profiles")
	if ctx.Run().FileExists(v23ProfilesFile) && !ctx.Run().FileExists(jiriProfilesFile) {
		if err := ctx.Run().Rename(v23ProfilesFile, jiriProfilesFile); err != nil {
			return fmt.Errorf("Rename(%v,%v) failed: %v", v23ProfilesFile, jiriProfilesFile, err)
		}
	}

	// Delete old "v23" tool, and the old jiri-xprofile command.
	// TODO(nlacasse): Once everybody has had a chance to update, remove this
	// code.
	v23SubCmds := []string{
		"jiri-xprofile",
		"v23",
	}
	for _, subCmd := range v23SubCmds {
		subCmdPath := filepath.Join(binDir, subCmd)
		if err := ctx.Run().RemoveAll(subCmdPath); err != nil {
			return err
		}
	}

	return nil
}

// runHooks runs the specified hooks
func runHooks(ctx *tool.Context, hooks Hooks) error {
	for _, hook := range hooks {
		command := hook.Path
		args := []string{}
		if hook.Interpreter != "" {
			command = hook.Interpreter
			args = append(args, hook.Path)
		}
		for _, arg := range hook.Args {
			args = append(args, arg.Arg)
		}
		if err := ctx.Run().Command(command, args...); err != nil {
			return fmt.Errorf("Hook %v failed: %v", hook.Name, err)
		}
	}
	return nil
}

// pullProject advances the local master branch of the given
// project, which is expected to exist locally at project.Path.
func pullProject(ctx *tool.Context, project Project) error {
	pullFn := func() error {
		switch project.Protocol {
		case "git":
			if err := ctx.Git().Pull("origin", "master"); err != nil {
				return err
			}
			return ctx.Git().Reset(project.Revision)
		default:
			return UnsupportedProtocolErr(project.Protocol)
		}
	}
	return ApplyToLocalMaster(ctx, project, pullFn)
}

// loadManifest loads the given manifest, processing all of its
// imports, projects and tools settings.
func loadManifest(ctx *tool.Context, path string, hosts Hosts, projects Projects, tools Tools, hooks Hooks, stack map[string]struct{}) error {
	data, err := ctx.Run().ReadFile(path)
	if err != nil {
		return err
	}
	m := &Manifest{}
	if err := xml.Unmarshal(data, m); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	// Process all imports.
	for _, manifest := range m.Imports {
		if _, ok := stack[manifest.Name]; ok {
			return fmt.Errorf("import cycle encountered")
		}
		path, err := ResolveManifestPath(manifest.Name)
		if err != nil {
			return err
		}
		stack[manifest.Name] = struct{}{}
		if err := loadManifest(ctx, path, hosts, projects, tools, hooks, stack); err != nil {
			return err
		}
		delete(stack, manifest.Name)
	}
	// Process all projects.
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	for _, project := range m.Projects {
		if project.Exclude {
			// Exclude the project in case it was
			// previously included.
			delete(projects, project.Name)
			continue
		}
		// Replace the relative path with an absolute one.
		project.Path = filepath.Join(root, project.Path)
		// Use git as the default protocol.
		if project.Protocol == "" {
			project.Protocol = "git"
		}
		// Use HEAD and tip as the default revision for git
		// and mercurial respectively.
		if project.Revision == "" {
			switch project.Protocol {
			case "git":
				project.Revision = "HEAD"
			default:
				return UnsupportedProtocolErr(project.Protocol)
			}
		}
		projects[project.Name] = project
	}
	// Process all tools.
	for _, tool := range m.Tools {
		if tool.Exclude {
			// Exclude the tool in case it was previously
			// included.
			delete(tools, tool.Name)
			continue
		}
		// Use <JiriProject> as the default project.
		if tool.Project == "" {
			tool.Project = "https://vanadium.googlesource.com/" + JiriProject
		}
		// Use "data" as the default data.
		if tool.Data == "" {
			tool.Data = "data"
		}
		tools[tool.Name] = tool
	}
	// Process all hooks.
	for _, hook := range m.Hooks {
		if hook.Exclude {
			// Exclude the hook in case it was previously
			// included.
			delete(hooks, hook.Name)
			continue
		}
		project, found := projects[hook.Project]
		if !found {
			return fmt.Errorf("hook %v specified project %v which was not found",
				hook.Name, hook.Project)
		}
		// Replace project-relative path with absolute path.
		hook.Path = filepath.Join(project.Path, hook.Path)
		hooks[hook.Name] = hook
	}
	// Process all hosts.
	for _, host := range m.Hosts {
		hosts[host.Name] = host

		// Sanity check that we only have githooks for git hosts.
		if host.Name != "git" {
			if len(host.GitHooks) > 0 {
				return fmt.Errorf("githook provided for a non-Git host: %s", host.Location)
			}
		}
	}

	return nil
}

// reportNonMaster checks if the given project is on master branch and
// if not, reports this fact along with information on how to update it.
func reportNonMaster(ctx *tool.Context, project Project) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(project.Path); err != nil {
		return err
	}
	switch project.Protocol {
	case "git":
		current, err := ctx.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		if current != "master" {
			line1 := fmt.Sprintf(`NOTE: "jiri update" only updates the "master" branch and the current branch is %q`, current)
			line2 := fmt.Sprintf(`to update the %q branch once the master branch is updated, run "git merge master"`, current)
			opts := runutil.Opts{Verbose: true}
			ctx.Run().OutputWithOpts(opts, []string{line1, line2})
		}
		return nil
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
}

// snapshotLocalProjects returns an in-memory representation of the
// current state of all local projects
func snapshotLocalProjects(ctx *tool.Context) (*Manifest, error) {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	root, err := JiriRoot()
	if err != nil {
		return nil, err
	}
	manifest := Manifest{}
	for _, project := range localProjects {
		revision := ""
		revisionFn := func() error {
			switch project.Protocol {
			case "git":
				gitRevision, err := ctx.Git().CurrentRevision()
				if err != nil {
					return err
				}
				revision = gitRevision
				return nil
			default:
				return UnsupportedProtocolErr(project.Protocol)
			}
		}
		if err := ApplyToLocalMaster(ctx, project, revisionFn); err != nil {
			return nil, err
		}
		project.Revision = revision
		project.Path = strings.TrimPrefix(project.Path, root)
		manifest.Projects = append(manifest.Projects, project)
	}
	return &manifest, nil
}

// updateProjects updates all jiri projects.
func updateProjects(ctx *tool.Context, remoteProjects Projects, gc bool) error {
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	localManifest, err := snapshotLocalProjects(ctx)
	if err != nil {
		return err
	}
	localProjects := make(Projects)
	for _, project := range localManifest.Projects {
		project.Path = filepath.Join(root, project.Path)
		localProjects[project.Name] = project
	}
	gitHost, gitHostErr := GitHost(ctx)
	if gitHostErr == nil && googlesource.IsGoogleSourceHost(gitHost) {
		// Attempt to get the repo statuses from remote so we can detect when a
		// local project is already up-to-date.
		if repoStatuses, err := googlesource.GetRepoStatuses(gitHost); err != nil {
			// Log the error but don't fail.
			fmt.Fprintf(ctx.Stderr(), "Error fetching repo statuses from remote: %v\n", err)
		} else {
			for name, rp := range remoteProjects {
				status, ok := repoStatuses[rp.Name]
				if !ok {
					continue
				}
				masterRev, ok := status.Branches["master"]
				if !ok || masterRev == "" {
					continue
				}
				rp.Revision = masterRev
				remoteProjects[name] = rp
			}
		}
	}

	ops, err := computeOperations(localProjects, remoteProjects, gc)
	if err != nil {
		return err
	}

	for _, op := range ops {
		if err := op.Test(ctx); err != nil {
			return err
		}
	}
	failed := false
	manifest := &Manifest{Label: ctx.Manifest()}
	for _, op := range ops {
		updateFn := func() error { return op.Run(ctx, manifest) }
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		opts := runutil.Opts{Verbose: true}
		if err := ctx.Run().FunctionWithOpts(opts, updateFn, "%v", op); err != nil {
			fmt.Fprintf(ctx.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	if err := writeCurrentManifest(ctx, manifest); err != nil {
		return err
	}
	return nil
}

// writeMetadata stores the given project metadata in the directory
// identified by the given path.
func writeMetadata(ctx *tool.Context, project Project, dir string) (e error) {
	metadataDir := filepath.Join(dir, metadataDirName)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().MkdirAll(metadataDir, os.FileMode(0755)); err != nil {
		return err
	}
	if err := ctx.Run().Chdir(metadataDir); err != nil {
		return err
	}
	// Replace absolute project paths with relative paths to make it
	// possible to move the $JIRI_ROOT directory locally.
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	project.Path = strings.TrimPrefix(project.Path, root)
	bytes, err := xml.Marshal(project)
	if err != nil {
		return fmt.Errorf("Marhsal() failed: %v", err)
	}
	metadataFile := filepath.Join(metadataDir, metadataFileName)
	tmpMetadataFile := metadataFile + ".tmp"
	if err := ctx.Run().WriteFile(tmpMetadataFile, bytes, os.FileMode(0644)); err != nil {
		return err
	}
	if err := ctx.Run().Rename(tmpMetadataFile, metadataFile); err != nil {
		return err
	}
	return nil
}

// addProjectToManifest records the information about the given
// project in the given manifest. The function is used to create a
// manifest that records the current state of jiri projects, which
// can be used to restore this state at some later point.
//
// NOTE: The function assumes that the the given project is on a
// master branch.
func addProjectToManifest(ctx *tool.Context, manifest *Manifest, project Project) error {
	// If the project uses relative revision, replace it with an absolute one.
	switch project.Protocol {
	case "git":
		if project.Revision == "HEAD" {
			revision, err := ctx.Git(tool.RootDirOpt(project.Path)).CurrentRevision()
			if err != nil {
				return err
			}
			project.Revision = revision
		}
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
	// Replace absolute path with a relative one.
	root, err := JiriRoot()
	if err != nil {
		return err
	}
	project.Path = strings.TrimPrefix(project.Path, root+string(filepath.Separator))
	manifest.Projects = append(manifest.Projects, project)
	return nil
}

type operation interface {
	// Project identifies the project this operation pertains to.
	Project() Project
	// Run executes the operation.
	Run(ctx *tool.Context, manifest *Manifest) error
	// String returns a string representation of the operation.
	String() string
	// Test checks whether the operation would fail.
	Test(ctx *tool.Context) error
}

// commonOperation represents a project operation.
type commonOperation struct {
	// project holds information about the project such as its
	// name, local path, and the protocol it uses for version
	// control.
	project Project
	// destination is the new project path.
	destination string
	// source is the current project path.
	source string
}

func (commonOperation) Run(*tool.Context, *Manifest) error {
	return nil
}

func (op commonOperation) Project() Project {
	return op.project
}

func (commonOperation) String() string {
	return ""
}

func (commonOperation) Test(*tool.Context) error {
	return nil
}

// createOperation represents the creation of a project.
type createOperation struct {
	commonOperation
}

func (op createOperation) Run(ctx *tool.Context, manifest *Manifest) (e error) {
	hosts, _, _, _, err := readManifest(ctx, false)
	if err != nil {
		return err
	}

	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	// Create a temporary directory for the initial setup of the
	// project to prevent an untimely termination from leaving the
	// $JIRI_ROOT directory in an inconsistent state.
	tmpDir, err := ctx.Run().TempDir(path, "")
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
	switch op.project.Protocol {
	case "git":
		if err := ctx.Git().Clone(op.project.Remote, tmpDir); err != nil {
			return err
		}

		// Apply git hooks.  We're creating this repo, so there's no danger of
		// overriding existing hooks.  Customizing your git hooks with jiri is a bad
		// idea anyway, since jiri won't know to not delete the project when you
		// switch between manifests or do a cleanup.
		host, found := hosts["git"]
		if found && strings.HasPrefix(op.project.Remote, host.Location) {
			gitHookDir := filepath.Join(tmpDir, ".git", "hooks")
			for _, githook := range host.GitHooks {
				mdir, err := ManifestDir()
				if err != nil {
					return err
				}
				src, err := ctx.Run().ReadFile(filepath.Join(mdir, githook.Path))
				if err != nil {
					return err
				}
				dst := filepath.Join(gitHookDir, githook.Name)
				if err := ctx.Run().WriteFile(dst, src, perm); err != nil {
					return err
				}
			}
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
		if err := ctx.Run().Chdir(tmpDir); err != nil {
			return err
		}
		if err := ctx.Git().Reset(op.project.Revision); err != nil {
			return err
		}
	default:
		return UnsupportedProtocolErr(op.project.Protocol)
	}
	if err := writeMetadata(ctx, op.project, tmpDir); err != nil {
		return err
	}
	if err := ctx.Run().Chmod(tmpDir, os.FileMode(0755)); err != nil {
		return err
	}
	if err := ctx.Run().Rename(tmpDir, op.destination); err != nil {
		return err
	}
	if err := addProjectToManifest(ctx, manifest, op.project); err != nil {
		return err
	}
	return nil
}

func (op createOperation) String() string {
	return fmt.Sprintf("create project %q in %q and advance it to %q", op.project.Name, op.destination, op.project.Revision)
}

func (op createOperation) Test(ctx *tool.Context) error {
	// Check the local file system.
	if _, err := ctx.Run().Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		return fmt.Errorf("cannot create %q as it already exists", op.destination)
	}
	return nil
}

// deleteOperation represents the deletion of a project.
type deleteOperation struct {
	commonOperation
	// gc determines whether the operation should be executed or
	// whether it should only print a notification.
	gc bool
}

func (op deleteOperation) Run(ctx *tool.Context, _ *Manifest) error {
	if op.gc {
		// Never delete the <JiriProject>.
		if op.project.Name == JiriProject {
			lines := []string{
				fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
				"however this project is required for correct operation of the jiri",
				"development tools and will thus not be deleted",
			}
			opts := runutil.Opts{Verbose: true}
			ctx.Run().OutputWithOpts(opts, lines)
			return nil
		}
		// Never delete projects with non-master branches, uncommitted
		// work, or untracked content.
		git := ctx.Git(tool.RootDirOpt(op.project.Path))
		branches, _, err := git.GetBranches()
		if err != nil {
			return err
		}
		uncommitted, err := git.HasUncommittedChanges()
		if err != nil {
			return err
		}
		untracked, err := git.HasUntrackedFiles()
		if err != nil {
			return err
		}
		if len(branches) != 1 || uncommitted || untracked {
			lines := []string{
				fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
				"however this project either contains non-master branches, uncommitted",
				"work, or untracked files and will thus not be deleted",
			}
			opts := runutil.Opts{Verbose: true}
			ctx.Run().OutputWithOpts(opts, lines)
			return nil
		}
		return ctx.Run().RemoveAll(op.source)
	}
	lines := []string{
		fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
		"it was not automatically removed to avoid deleting uncommitted work",
		fmt.Sprintf(`if you no longer need it, invoke "rm -rf %v"`, op.source),
		`or invoke "jiri update -gc" to remove all such local projects`,
	}
	opts := runutil.Opts{Verbose: true}
	ctx.Run().OutputWithOpts(opts, lines)
	return nil
}

func (op deleteOperation) String() string {
	return fmt.Sprintf("delete project %q from %q", op.project.Name, op.source)
}

func (op deleteOperation) Test(ctx *tool.Context) error {
	if _, err := ctx.Run().Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot delete %q as it does not exist", op.source)
		}
		return err
	}
	return nil
}

// moveOperation represents the relocation of a project.
type moveOperation struct {
	commonOperation
}

func (op moveOperation) Run(ctx *tool.Context, manifest *Manifest) error {
	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	if err := ctx.Run().Rename(op.source, op.destination); err != nil {
		return err
	}
	if err := reportNonMaster(ctx, op.project); err != nil {
		return err
	}
	if err := pullProject(ctx, op.project); err != nil {
		return err
	}
	if err := writeMetadata(ctx, op.project, op.project.Path); err != nil {
		return err
	}
	if err := addProjectToManifest(ctx, manifest, op.project); err != nil {
		return err
	}
	return nil
}

func (op moveOperation) String() string {
	return fmt.Sprintf("move project %q located in %q to %q and advance it to %q", op.project.Name, op.source, op.destination, fmtRevision(op.project.Revision))
}

func (op moveOperation) Test(ctx *tool.Context) error {
	if _, err := ctx.Run().Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
		}
		return err
	}
	if _, err := ctx.Run().Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		return fmt.Errorf("cannot move %q to %q as the destination already exists", op.source, op.destination)
	}
	return nil
}

// updateOperation represents the update of a project.
type updateOperation struct {
	commonOperation
}

func (op updateOperation) Run(ctx *tool.Context, manifest *Manifest) error {
	if err := reportNonMaster(ctx, op.project); err != nil {
		return err
	}
	if err := pullProject(ctx, op.project); err != nil {
		return err
	}
	if err := writeMetadata(ctx, op.project, op.project.Path); err != nil {
		return err
	}
	if err := addProjectToManifest(ctx, manifest, op.project); err != nil {
		return err
	}
	return nil
}

func (op updateOperation) String() string {
	return fmt.Sprintf("advance project %q located in %q to %q", op.project.Name, op.source, fmtRevision(op.project.Revision))
}

func (op updateOperation) Test(ctx *tool.Context) error {
	return nil
}

// nullOperation represents a noop.  Used only for logging.
type nullOperation struct {
	commonOperation
}

func (op nullOperation) Run(ctx *tool.Context, manifest *Manifest) error {
	return nil
}

func (op nullOperation) String() string {
	return fmt.Sprintf("project %q located in %q at revision %q is up-to-date", op.project.Name, op.source, fmtRevision(op.project.Revision))
}

func (op nullOperation) Test(ctx *tool.Context) error {
	return nil
}

// operations is a sortable collection of operations
type operations []operation

// Len returns the length of the collection.
func (ops operations) Len() int {
	return len(ops)
}

// Less defines the order of operations. Operations are ordered first
// by their type and then by their project name.
//
// The order in which operation types are defined determines the order
// in which operations are performed. For correctness and also to
// minimize the chance of a conflict, the delete operations should
// happen before move operations, which should happen before create
// operations.
func (ops operations) Less(i, j int) bool {
	vals := make([]int, 2)
	for idx, op := range []operation{ops[i], ops[j]} {
		switch op.(type) {
		case deleteOperation:
			vals[idx] = 0
		case moveOperation:
			vals[idx] = 1
		case createOperation:
			vals[idx] = 2
		case updateOperation:
			vals[idx] = 3
		case nullOperation:
			vals[idx] = 4
		}
	}
	if vals[0] != vals[1] {
		return vals[0] < vals[1]
	}
	return ops[i].Project().Name < ops[j].Project().Name
}

// Swap swaps two elements of the collection.
func (ops operations) Swap(i, j int) {
	ops[i], ops[j] = ops[j], ops[i]
}

// computeOperations inputs a set of projects to update and the set of
// current and new projects (as defined by contents of the local file
// system and manifest file respectively) and outputs a collection of
// operations that describe the actions needed to update the target
// projects.
func computeOperations(localProjects, remoteProjects Projects, gc bool) (operations, error) {
	result := operations{}
	allProjects := map[string]struct{}{}
	for name, _ := range localProjects {
		allProjects[name] = struct{}{}
	}
	for name, _ := range remoteProjects {
		allProjects[name] = struct{}{}
	}
	for name, _ := range allProjects {
		if localProject, ok := localProjects[name]; ok {
			if remoteProject, ok := remoteProjects[name]; ok {
				if localProject.Path != remoteProject.Path {
					// moveOperation also does an update, so we don't need to
					// check the revision here.
					result = append(result, moveOperation{commonOperation{
						destination: remoteProject.Path,
						project:     remoteProject,
						source:      localProject.Path,
					}})
				} else {
					if localProject.Revision != remoteProject.Revision {
						result = append(result, updateOperation{commonOperation{
							destination: remoteProject.Path,
							project:     remoteProject,
							source:      localProject.Path,
						}})
					} else {
						result = append(result, nullOperation{commonOperation{
							destination: remoteProject.Path,
							project:     remoteProject,
							source:      localProject.Path,
						}})
					}
				}
			} else {
				result = append(result, deleteOperation{commonOperation{
					destination: "",
					project:     localProject,
					source:      localProject.Path,
				}, gc})
			}
		} else if remoteProject, ok := remoteProjects[name]; ok {
			result = append(result, createOperation{commonOperation{
				destination: remoteProject.Path,
				project:     remoteProject,
				source:      "",
			}})
		} else {
			return nil, fmt.Errorf("project %v does not exist", name)
		}
	}
	sort.Sort(result)
	return result, nil
}

// ParseNames identifies the set of projects that a jiri command should
// be applied to.
func ParseNames(ctx *tool.Context, args []string, defaultProjects map[string]struct{}) (map[string]Project, error) {
	projects, _, err := ReadManifest(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]Project{}
	if len(args) == 0 {
		// Use the default set of projects.
		args = set.String.ToSlice(defaultProjects)
	}
	for _, name := range args {
		if project, ok := projects[name]; ok {
			result[name] = project
		} else {
			// Issue a warning if the target project does not exist in the
			// project manifest.
			fmt.Fprintf(ctx.Stderr(), "WARNING: project %q does not exist in the project manifest and will be skipped\n", name)
		}
	}
	return result, nil
}

// fmtRevision returns the first 8 chars of a revision hash.
func fmtRevision(r string) string {
	l := 8
	if len(r) < l {
		return r
	}
	return r[:l]
}
