package util

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
)

type FakeVanadiumRoot struct {
	Dir      string
	Projects map[string]string
	remote   string
}

const (
	defaultDataDir    = "data"
	defaultConfigFile = "conf.json"
	defaultManifest   = "default"
	manifestProject   = ".manifest"
	manifestVersion   = "v2"
	toolsProject      = "tools"
)

// NewFakeVanadiumRoot is the FakeVanadiumRoot factory.
func NewFakeVanadiumRoot(ctx *tool.Context) (*FakeVanadiumRoot, error) {
	root := &FakeVanadiumRoot{
		Projects: map[string]string{},
	}

	// Create fake remote manifest and tools projects.
	remoteDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, err
	}
	root.remote = remoteDir
	if err := root.CreateRemoteProject(ctx, manifestProject); err != nil {
		return nil, err
	}
	if err := root.CreateRemoteProject(ctx, toolsProject); err != nil {
		return nil, err
	}

	// Create a fake configuration file.
	toolsConfigPath := filepath.Join(remoteDir, toolsProject, defaultDataDir)
	if err := ctx.Run().MkdirAll(toolsConfigPath, os.FileMode(0700)); err != nil {
		return nil, err
	}
	var config Config
	if err := root.WriteRemoteToolsConfig(ctx, &config); err != nil {
		return nil, err
	}

	// Create a fake manifest.
	manifestDir := filepath.Join(remoteDir, manifestProject, manifestVersion)
	if err := ctx.Run().MkdirAll(manifestDir, os.FileMode(0700)); err != nil {
		return nil, err
	}
	if err := root.WriteRemoteManifest(ctx, &Manifest{}); err != nil {
		return nil, err
	}

	// Create a fake VANADIUM_ROOT.
	rootDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, err
	}
	root.Dir = rootDir
	if err := ctx.Git().Clone(root.Projects[manifestProject], filepath.Join(root.Dir, manifestProject)); err != nil {
		return nil, err
	}

	// Add the "tools" project and a fake "v23" tool to the
	// manifests. This is necessary to make sure that the commonly
	// invoked ConfigPath() function, which requires every manifest
	// contains a "v23" tool, works.
	if err := root.AddProject(ctx, Project{
		Name:   toolsProject,
		Path:   toolsProject,
		Remote: root.Projects[toolsProject],
	}); err != nil {
		return nil, err
	}
	if err := root.AddTool(ctx, Tool{
		Name:    "v23",
		Data:    defaultDataDir,
		Project: toolsProject,
	}); err != nil {
		return nil, err
	}

	// Update the contents of the fake VANADIUM_ROOT instance based on
	// the information recorded in the remote manifest.
	if err := root.UpdateUniverse(ctx, false); err != nil {
		return nil, err
	}

	return root, nil
}

// Cleanup cleans up the given Vanadium root fake.
func (root FakeVanadiumRoot) Cleanup(ctx *tool.Context) error {
	var errs []error
	collect.Errors(func() error { return ctx.Run().RemoveAll(root.Dir) }, &errs)
	collect.Errors(func() error { return ctx.Run().RemoveAll(root.remote) }, &errs)
	if len(errs) != 0 {
		return fmt.Errorf("Cleanup() failed: %v", errs)
	}
	return nil
}

// AddProject adds the given project to a remote manifest.
func (root FakeVanadiumRoot) AddProject(ctx *tool.Context, project Project) error {
	manifest, err := root.ReadRemoteManifest(ctx)
	if err != nil {
		return err
	}
	manifest.Projects = append(manifest.Projects, project)
	if err := root.WriteRemoteManifest(ctx, manifest); err != nil {
		return err
	}
	return nil
}

// AddTool adds the given tool to a remote manifest.
func (root FakeVanadiumRoot) AddTool(ctx *tool.Context, tool Tool) error {
	manifest, err := root.ReadRemoteManifest(ctx)
	if err != nil {
		return err
	}
	manifest.Tools = append(manifest.Tools, tool)
	if err := root.WriteRemoteManifest(ctx, manifest); err != nil {
		return err
	}
	return nil
}

// DisableRemoteManifestPush disables pushes to the remote manifest
// repository.
func (root FakeVanadiumRoot) DisableRemoteManifestPush(ctx *tool.Context) error {
	dir := tool.RootDirOpt(filepath.Join(root.remote, manifestProject))
	if err := ctx.Git(dir).CheckoutBranch("master", false); err != nil {
		return err
	}
	return nil
}

// EnableRemoteManifestPush enables pushes to the remote manifest
// repository.
func (root FakeVanadiumRoot) EnableRemoteManifestPush(ctx *tool.Context) error {
	dir := tool.RootDirOpt(filepath.Join(root.remote, manifestProject))
	if !ctx.Git(dir).BranchExists("non-master") {
		if err := ctx.Git(dir).CreateBranch("non-master"); err != nil {
			return err
		}
	}
	if err := ctx.Git(dir).CheckoutBranch("non-master", false); err != nil {
		return err
	}
	return nil
}

// CreateRemoteProject creates a new remote project.
func (root FakeVanadiumRoot) CreateRemoteProject(ctx *tool.Context, name string) error {
	projectDir := filepath.Join(root.remote, name)
	if err := ctx.Run().MkdirAll(projectDir, os.FileMode(0700)); err != nil {
		return err
	}
	if err := ctx.Git().Init(projectDir); err != nil {
		return err
	}
	if err := ctx.Git(tool.RootDirOpt(projectDir)).CommitWithMessage("initial commit"); err != nil {
		return err
	}
	root.Projects[name] = projectDir
	return nil
}

func getManifest(ctx *tool.Context) string {
	manifest := ctx.Manifest()
	if manifest != "" {
		return manifest
	}
	return defaultManifest
}

// ReadLocalToolsConfig reads a tools configuration from the local
// tools project.
func (root FakeVanadiumRoot) ReadLocalToolsConfig(ctx *tool.Context) (*Config, error) {
	path := filepath.Join(root.Dir, toolsProject, defaultDataDir, defaultConfigFile)
	return root.readToolsConfig(ctx, path)
}

// ReadLocalToolsConfig reads a tools configuration from the remote
// tools project.
func (root FakeVanadiumRoot) ReadRemoteToolsConfig(ctx *tool.Context) (*Config, error) {
	path := filepath.Join(root.remote, toolsProject, defaultDataDir, defaultConfigFile)
	return root.readToolsConfig(ctx, path)
}

func (root FakeVanadiumRoot) readToolsConfig(ctx *tool.Context, path string) (*Config, error) {
	bytes, err := ctx.Run().ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := xml.Unmarshal(bytes, &config); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	return &config, nil
}

// ReadLocalManifest read a manifest from the local manifest project.
func (root FakeVanadiumRoot) ReadLocalManifest(ctx *tool.Context) (*Manifest, error) {
	path := filepath.Join(root.Dir, manifestProject, manifestVersion, getManifest(ctx))
	return root.readManifest(ctx, path)
}

// ReadRemoteManifest read a manifest from the remote manifest project.
func (root FakeVanadiumRoot) ReadRemoteManifest(ctx *tool.Context) (*Manifest, error) {
	path := filepath.Join(root.remote, manifestProject, manifestVersion, getManifest(ctx))
	return root.readManifest(ctx, path)
}

func (root FakeVanadiumRoot) readManifest(ctx *tool.Context, path string) (*Manifest, error) {
	bytes, err := ctx.Run().ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := xml.Unmarshal(bytes, &manifest); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	return &manifest, nil
}

// UpdateUniverse synchronizes the content of the Vanadium root based
// on the content of the remote manifest.
func (root FakeVanadiumRoot) UpdateUniverse(ctx *tool.Context, gc bool) error {
	oldRoot := os.Getenv("VANADIUM_ROOT")
	if err := os.Setenv("VANADIUM_ROOT", root.Dir); err != nil {
		return fmt.Errorf("Setenv() failed: %v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)
	if err := UpdateUniverse(ctx, gc); err != nil {
		return err
	}
	return nil
}

// WriteLocalToolsConfig writes the given tools configuration to the
// local tools project.
func (root FakeVanadiumRoot) WriteLocalToolsConfig(ctx *tool.Context, config *Config) error {
	dir := filepath.Join(root.Dir, toolsProject)
	path := filepath.Join(dir, defaultDataDir, defaultConfigFile)
	return root.writeToolsConfig(ctx, config, dir, path)
}

// WriteRemoteToolsConfig writes the given tools configuration to the
// remote tools project.
func (root FakeVanadiumRoot) WriteRemoteToolsConfig(ctx *tool.Context, config *Config) error {
	dir := filepath.Join(root.remote, toolsProject)
	path := filepath.Join(dir, defaultDataDir, defaultConfigFile)
	return root.writeToolsConfig(ctx, config, dir, path)
}

func (root FakeVanadiumRoot) writeToolsConfig(ctx *tool.Context, config *Config, dir, path string) error {
	bytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", config, err)
	}
	if err := ctx.Run().WriteFile(path, bytes, os.FileMode(0600)); err != nil {
		return err
	}
	if err := ctx.Git(tool.RootDirOpt(dir)).Add(path); err != nil {
		return err
	}
	if err := ctx.Git(tool.RootDirOpt(dir)).Commit(); err != nil {
		return err
	}
	return nil
}

// WriteLocalManifest writes the given manifest to the local
// manifest project.
func (root FakeVanadiumRoot) WriteLocalManifest(ctx *tool.Context, manifest *Manifest) error {
	dir := filepath.Join(root.Dir, manifestProject)
	path := filepath.Join(dir, manifestVersion, getManifest(ctx))
	return root.writeManifest(ctx, manifest, dir, path)
}

// WriteRemoteManifest writes the given manifest to the remote
// manifest project.
func (root FakeVanadiumRoot) WriteRemoteManifest(ctx *tool.Context, manifest *Manifest) error {
	dir := filepath.Join(root.remote, manifestProject)
	path := filepath.Join(dir, manifestVersion, getManifest(ctx))
	return root.writeManifest(ctx, manifest, dir, path)
}

func (root FakeVanadiumRoot) writeManifest(ctx *tool.Context, manifest *Manifest, dir, path string) error {
	bytes, err := xml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", manifest, err)
	}
	if err := ctx.Run().WriteFile(path, bytes, os.FileMode(0600)); err != nil {
		return err
	}
	if err := ctx.Git(tool.RootDirOpt(dir)).Add(path); err != nil {
		return err
	}
	if err := ctx.Git(tool.RootDirOpt(dir)).Commit(); err != nil {
		return err
	}
	return nil
}
