// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

// CL represents a changelist.
type CL struct {
	// Author identifies the author of the changelist.
	Author string
	// Email identifies the author's email.
	Email string
	// Description holds the description of the changelist.
	Description string
}

// Manifest represents a setting used for updating the vanadium universe.
type Manifest struct {
	Imports  []Import  `xml:"imports>import"`
	Projects []Project `xml:"projects>project"`
	Tools    []Tool    `xml:"tools>tool"`
	XMLName  struct{}  `xml:"manifest"`
}

// Imports maps manifest import names to their detailed description.
type Imports map[string]Import

// Import represnts a manifest import.
type Import struct {
	// Name is the name under which the manifest can be found the
	// manifest repository.
	Name string `xml:"name,attr"`
}

// Projects maps vanadium project names to their detailed description.
type Projects map[string]Project

// Project represents a vanadium project.
type Project struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Name is the project name.
	Name string `xml:"name,attr"`
	// Path is the path used to store the project locally. Project
	// manifest uses paths that are relative to the V23_ROOT
	// environment variable. When a manifest is parsed (e.g. in
	// RemoteProjects), the program logic converts the relative
	// paths to an absolute paths, using the current value of the
	// V23_ROOT environment variable as a prefix.
	Path string `xml:"path,attr"`
	// Protocol is the version control protocol used by the
	// project. If not set, "git" is used as the default.
	Protocol string `xml:"protocol,attr"`
	// Remote is the project remote.
	Remote string `xml:"remote,attr"`
	// Revision is the revision the project should be advanced to
	// during "v23 update". If not set, "HEAD" is used as the
	// default.
	Revision string `xml:"revision,attr"`
}

// Tools maps vanadium tool names, to their detailed description.
type Tools map[string]Tool

// Tool represents a vanadium tool.
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
	// set, "https://vanadium.googlesource.com/release.go.tools" is used
	// as the default.
	Project string `xml:"project,attr"`
}

type UnsupportedProtocolErr string

func (e UnsupportedProtocolErr) Error() string {
	return fmt.Sprintf("unsupported protocol %v", e)
}

// Update represents an update of vanadium projects as a map from
// project names to a collections of commits.
type Update map[string][]CL

// CreateSnapshot creates a manifest that encodes the current state of
// master branches of all projects and writes this snapshot out to the
// given file.
func CreateSnapshot(ctx *tool.Context, path string) error {
	// Create an in-memory representation of the build manifest.
	manifest, err := snapshotLocalProjects(ctx)
	if err != nil {
		return err
	}
	perm := os.FileMode(0755)
	if err := ctx.Run().MkdirAll(filepath.Dir(path), perm); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	perm = os.FileMode(0644)
	if err := ctx.Run().WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("WriteFile(%v, %v) failed: %v", path, err, perm)
	}
	return nil
}

// CurrentProjectName gets the name of the current project from the
// current directory by reading the .v23/metadata.v2 file located at
// the root of the current repository.
func CurrentProjectName(ctx *tool.Context) (string, error) {
	topLevel, err := ctx.Git().TopLevel()
	if err != nil {
		return "", nil
	}
	v23Dir := filepath.Join(topLevel, ".v23")
	if _, err := os.Stat(v23Dir); err == nil {
		metadataFile := filepath.Join(v23Dir, metadataFileName)
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

// LocalProjects scans the local filesystem to identify existing
// projects.
func LocalProjects(ctx *tool.Context) (Projects, error) {
	root, err := V23Root()
	if err != nil {
		return nil, err
	}
	projects := Projects{}
	if err := findLocalProjects(ctx, root, projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// PollProjects returns the set of changelists that exist remotely but
// not locally. Changes are grouped by vanadium projects and contain
// author identification and a description of their content.
func PollProjects(ctx *tool.Context, manifest string, projectSet map[string]struct{}) (_ Update, e error) {
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
	remoteProjects, _, err := readManifest(ctx, false)
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
// projects and tools are part of the vanadium universe.
func ReadManifest(ctx *tool.Context) (Projects, Tools, error) {
	return readManifest(ctx, false)
}

// readManifest implements the ReadManifest logic and provides an
// optional flag that can be used to fetch the latest manifest updates
// from the manifest repository.
func readManifest(ctx *tool.Context, update bool) (Projects, Tools, error) {
	if update {
		root, err := V23Root()
		if err != nil {
			return nil, nil, err
		}
		project := Project{
			Path:     filepath.Join(root, ".manifest"),
			Protocol: "git",
			Revision: "HEAD",
		}
		if err := pullProject(ctx, project); err != nil {
			return nil, nil, err
		}
	}
	path, err := ResolveManifestPath(ctx.Manifest())
	if err != nil {
		return nil, nil, err
	}
	projects, tools, stack := Projects{}, Tools{}, map[string]struct{}{}
	if err := loadManifest(path, projects, tools, stack); err != nil {
		return nil, nil, err
	}
	return projects, tools, nil
}

// UpdateUniverse updates all local projects and tools to match the
// remote counterparts identified by the given manifest. Optionally,
// the 'gc' flag can be used to indicate that local projects that no
// longer exist remotely should be removed.
func UpdateUniverse(ctx *tool.Context, gc bool) (e error) {
	remoteProjects, remoteTools, err := readManifest(ctx, true)
	if err != nil {
		return err
	}
	// 1. Update all local projects to match their remote counterparts.
	if err := updateProjects(ctx, remoteProjects, gc); err != nil {
		return err
	}
	// 2. Build all tools in a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "tmp-vanadium-tools-build")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
	if err := buildTools(ctx, remoteTools, tmpDir); err != nil {
		return err
	}
	// 3. Install the tools into $V23_ROOT/devtools/bin.
	return installTools(ctx, tmpDir)
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
		if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Git().CheckoutBranch(branch, !gitutil.Force) }, &e)
	case "hg":
		branch, err := ctx.Hg().CurrentBranchName()
		if err != nil {
			return err
		}
		if err := ctx.Hg().CheckoutBranch("default"); err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Hg().CheckoutBranch(branch) }, &e)
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

	env, err := VanadiumEnvironment(ctx, HostPlatform())
	if err != nil {
		return err
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
	ldflags := fmt.Sprintf("-X v.io/x/devtools/internal/tool.Name %s -X v.io/x/devtools/internal/tool.Version %d", name, count)
	args := []string{"build", "-ldflags", ldflags, "-o", output, pkg}
	var stderr bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Env = env.Map()
	opts.Stdout = ioutil.Discard
	opts.Stderr = &stderr
	if err := ctx.Run().CommandWithOpts(opts, "go", args...); err != nil {
		return fmt.Errorf("%v tool build failed\n%v", name, stderr.String())
	}
	return nil
}

// buildTools builds and installs all vanadium tools using the version
// available in the local master branch of the tools
// repository. Notably, this function does not perform any version
// control operation on the master branch.
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
		// robustness, this step also allow us to create V23_ROOT
		// fakes without having to provide an implementation for the "v23"
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

// CleanupProjects restores the given vanadium projects back to their master
// branches and gets rid of all the local changes. If "cleanupBranches" is true,
// it will also delete all the non-master branches.
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
		if err := ctx.Git().CheckoutBranch("master", gitutil.Force); err != nil {
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
			if err := ctx.Git().DeleteBranch(branch, gitutil.Force); err != nil {
				return nil
			}
		}
	}

	return nil
}

const metadataFileName = "metadata.v2"

// findLocalProjects implements LocalProjects.
func findLocalProjects(ctx *tool.Context, path string, projects Projects) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(path); err != nil {
		return err
	}
	root, err := V23Root()
	if err != nil {
		return err
	}
	v23Dir := filepath.Join(path, ".v23")
	if _, err := os.Stat(v23Dir); err == nil {
		metadataFile := filepath.Join(v23Dir, metadataFileName)
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
		// Root relative paths in the V23_ROOT.
		if !filepath.IsAbs(project.Path) {
			project.Path = filepath.Join(root, project.Path)
		}
		projects[project.Name] = project
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("Stat(%v) failed: %v", v23Dir, err)
	}
	fileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", path, err)
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() && !strings.HasPrefix(fileInfo.Name(), ".") {
			if err := findLocalProjects(ctx, filepath.Join(path, fileInfo.Name()), projects); err != nil {
				return err
			}
		}
	}
	return nil
}

// installTools installs the tools from the given directory into
// $V23_ROOT/devtools/bin.
func installTools(ctx *tool.Context, dir string) error {
	if ctx.DryRun() {
		// In "dry run" mode, no binaries are built.
		return nil
	}
	root, err := V23Root()
	if err != nil {
		return err
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", dir, err)
	}
	failed := false
	// TODO(jsimsa): Make sure the "$V23_ROOT/devtools/bin" directory
	// exists. This is for backwards compatibility for instances of
	// V23_ROOT that have been created before go/vcl/9511.
	binDir := filepath.Join(root, "devtools", "bin")
	if err := ctx.Run().MkdirAll(binDir, os.FileMode(0755)); err != nil {
		return err
	}
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
	// TODO(jsimsa): Make sure the "$V23_ROOT/bin" directory is removed,
	// forcing people to update their PATH. Remove this once all
	// instances of V23_ROOT has been updated past go/vcl/9511.
	oldBinDir := filepath.Join(root, "bin")
	if err := ctx.Run().RemoveAll(oldBinDir); err != nil {
		return err
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
		case "hg":
			if err := ctx.Hg().Pull(); err != nil {
				return err
			}
			return ctx.Hg().CheckoutRevision(project.Revision)
		default:
			return UnsupportedProtocolErr(project.Protocol)
		}
	}
	return ApplyToLocalMaster(ctx, project, pullFn)
}

// loadManifest loads the given manifest, processing all of its
// imports, projects and tools settings.
func loadManifest(path string, projects Projects, tools Tools, stack map[string]struct{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ReadFile(%v) failed: %v", path, err)
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
		if err := loadManifest(path, projects, tools, stack); err != nil {
			return err
		}
		delete(stack, manifest.Name)
	}
	// Process all projects.
	root, err := V23Root()
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
			case "hg":
				project.Revision = "tip"
			default:
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
		// Use "release.go.x.devtools" as the default project.
		if tool.Project == "" {
			tool.Project = "https://vanadium.googlesource.com/release.go.x.devtools"
		}
		// Use "data" as the default data.
		if tool.Data == "" {
			tool.Data = "data"
		}
		tools[tool.Name] = tool
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
			line1 := fmt.Sprintf(`NOTE: "v23 update" only updates the "master" branch and the current branch is %q`, current)
			line2 := fmt.Sprintf(`to update the %q branch once the master branch is updated, run "git merge master"`, current)
			opts := runutil.Opts{Verbose: true}
			ctx.Run().OutputWithOpts(opts, []string{line1, line2})
		}
		return nil
	case "hg":
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
	root, err := V23Root()
	if err != nil {
		return nil, err
	}
	manifest := Manifest{}
	for _, project := range localProjects {
		revision := ""
		revisionFn := func() error {
			switch project.Protocol {
			case "git":
				gitRevision, err := ctx.Git().LatestCommitID()
				if err != nil {
					return err
				}
				revision = gitRevision
				return nil
			case "hg":
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

// updateProjects updates all vanadium projects.
func updateProjects(ctx *tool.Context, remoteProjects Projects, gc bool) error {
	localProjects, err := LocalProjects(ctx)
	if err != nil {
		return err
	}
	ops, err := computeOperations(localProjects, remoteProjects, gc)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if err := op.Test(); err != nil {
			return err
		}
	}
	failed := false
	for _, op := range ops {
		updateFn := func() error { return op.Run(ctx) }
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
	return nil
}

// writeMetadata stores the given project metadata in the directory
// identified by the given path.
func writeMetadata(ctx *tool.Context, project Project, dir string) (e error) {
	metadataDir := filepath.Join(dir, ".v23")
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
	// possible to move V23_ROOT locally.
	root, err := V23Root()
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

type operation interface {
	// Project identifies the project this operation pertains to.
	Project() Project
	// Run executes the operation.
	Run(ctx *tool.Context) error
	// String returns a string representation of the operation.
	String() string
	// Test checks whether the operation would fail.
	Test() error
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

func (commonOperation) Run(*tool.Context) error {
	return nil
}

func (op commonOperation) Project() Project {
	return op.project
}

func (commonOperation) String() string {
	return ""
}

func (commonOperation) Test() error {
	return nil
}

// createOperation represents the creation of a project.
type createOperation struct {
	commonOperation
}

func (op createOperation) Run(ctx *tool.Context) (e error) {
	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	// Create a temporary directory for the initial setup of the
	// project to prevent an untimely termination from leaving the
	// V23_ROOT in an inconsistent state.
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
		if strings.HasPrefix(op.project.Remote, VanadiumGitRepoHost()) {
			// Setup the repository for Gerrit code reviews.
			//
			// TODO(jsimsa): Decide what to do in case we would want to update the
			// commit hooks for existing repositories. Overwriting the existing
			// hooks is not a good idea as developers might have customized the
			// hooks.
			file := filepath.Join(tmpDir, ".git", "hooks", "commit-msg")
			if err := ctx.Run().WriteFile(file, []byte(commitMsgHook), perm); err != nil {
				return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
			}
			file = filepath.Join(tmpDir, ".git", "hooks", "pre-commit")
			if err := ctx.Run().WriteFile(file, []byte(preCommitHook), perm); err != nil {
				return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
			}
			file = filepath.Join(tmpDir, ".git", "hooks", "pre-push")
			if err := ctx.Run().WriteFile(file, []byte(prePushHook), perm); err != nil {
				return fmt.Errorf("WriteFile(%v, %v) failed: %v", file, perm, err)
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
	case "hg":
		if err := ctx.Hg().Clone(op.project.Remote, tmpDir); err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
		if err := ctx.Run().Chdir(tmpDir); err != nil {
			return err
		}
		if err := ctx.Hg().CheckoutRevision(op.project.Revision); err != nil {
			return err
		}
	default:
		return UnsupportedProtocolErr(op.project.Protocol)
	}
	if err := writeMetadata(ctx, op.project, tmpDir); err != nil {
		return err
	}
	if err := ctx.Run().Rename(tmpDir, op.destination); err != nil {
		return err
	}
	return nil
}

func (op createOperation) String() string {
	return fmt.Sprintf("create project %q in %q and advance it to %q", op.project.Name, op.destination, op.project.Revision)
}

func (op createOperation) Test() error {
	// Check the local file system.
	if _, err := os.Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
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

func (op deleteOperation) Run(ctx *tool.Context) error {
	if op.gc {
		return ctx.Run().RemoveAll(op.source)
	}
	lines := []string{
		fmt.Sprintf("NOTE: this project was not found in the project manifest"),
		"it was not automatically removed to avoid deleting uncommitted work",
		fmt.Sprintf(`if you no longer need it, invoke "rm -rf %v"`, op.source),
		`or invoke "v23 update -gc" to remove all such local projects`,
	}
	opts := runutil.Opts{Verbose: true}
	ctx.Run().OutputWithOpts(opts, lines)
	return nil
}

func (op deleteOperation) String() string {
	return fmt.Sprintf("delete project %q from %q", op.project.Name, op.source)
}

func (op deleteOperation) Test() error {
	if _, err := os.Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot delete %q as it does not exist", op.source)
		}
		return fmt.Errorf("Stat(%v) failed: %v", op.source, err)
	}
	return nil
}

// moveOperation represents the relocation of a project.
type moveOperation struct {
	commonOperation
}

func (op moveOperation) Run(ctx *tool.Context) error {
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
	return nil
}

func (op moveOperation) String() string {
	return fmt.Sprintf("move project %q located in %q to %q and advance it to %q", op.project.Name, op.source, op.destination, op.project.Revision)
}

func (op moveOperation) Test() error {
	if _, err := os.Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
		}
		return fmt.Errorf("Stat(%v) failed: %v", op.source, err)
	}
	if _, err := os.Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", op.destination, err)
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

func (op updateOperation) Run(ctx *tool.Context) error {
	if err := reportNonMaster(ctx, op.project); err != nil {
		return err
	}
	if err := pullProject(ctx, op.project); err != nil {
		return err
	}
	if err := writeMetadata(ctx, op.project, op.project.Path); err != nil {
		return err
	}
	return nil
}

func (op updateOperation) String() string {
	return fmt.Sprintf("advance project %q located in %q to %q", op.project.Name, op.source, op.project.Revision)
}

func (op updateOperation) Test() error {
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
				if localProject.Path == remoteProject.Path {
					result = append(result, updateOperation{commonOperation{
						destination: remoteProject.Path,
						project:     remoteProject,
						source:      localProject.Path,
					}})
				} else {
					result = append(result, moveOperation{commonOperation{
						destination: remoteProject.Path,
						project:     remoteProject,
						source:      localProject.Path,
					}})
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
