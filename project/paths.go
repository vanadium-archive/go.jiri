// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri/tool"
)

const (
	rootEnv              = "JIRI_ROOT"
	metadataDirName      = ".jiri"
	metadataFileName     = "metadata.v2"
	metadataProfilesFile = ".jiri_profiles"
)

// DataDirPath returns the path to the data directory of the given tool.
func DataDirPath(ctx *tool.Context, toolName string) (string, error) {
	_, projects, tools, _, err := readManifest(ctx, false)
	if err != nil {
		return "", err
	}
	if toolName == "" {
		// If the tool name is not set, use "jiri" as the default. As a
		// consequence, any manifest is assumed to specify a "jiri" tool.
		toolName = "jiri"
	}
	tool, ok := tools[toolName]
	if !ok {
		return "", fmt.Errorf("tool %q not found in the manifest", toolName)
	}
	projectName := tool.Project
	project, ok := projects[projectName]
	if !ok {
		return "", fmt.Errorf("project %q not found in the manifest", projectName)
	}
	return filepath.Join(project.Path, tool.Data), nil
}

// LocalManifestFile returns the path to the local manifest.
func LocalManifestFile() (string, error) {
	root, err := JiriRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".local_manifest"), nil
}

// LocalSnapshotDir returns the path to the local snapshot directory.
func LocalSnapshotDir() (string, error) {
	root, err := JiriRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".snapshot"), nil
}

// ManifestDir returns the path to the manifest directory.
func ManifestDir() (string, error) {
	root, err := JiriRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".manifest", "v2"), nil
}

// ManifestFile returns the path to the manifest file with the given
// relative path.
func ManifestFile(name string) (string, error) {
	dir, err := ManifestDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// MetadataDir returns the name of the directory in which jiri stores
// project specific metadata.
func MetadataDirName() string {
	return metadataDirName
}

// RemoteSnapshotDir returns the path to the remote snapshot directory.
func RemoteSnapshotDir() (string, error) {
	manifestDir, err := ManifestDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(manifestDir, "snapshot"), nil
}

// ResolveManifestPath resolves the given manifest name to an absolute
// path in the local filesystem.
func ResolveManifestPath(name string) (string, error) {
	if name != "" {
		if filepath.IsAbs(name) {
			return name, nil
		}
		return ManifestFile(name)
	}
	path, err := LocalManifestFile()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return ResolveManifestPath("default")
		}
		return "", fmt.Errorf("Stat(%v) failed: %v", path, err)
	}
	return path, nil
}

func getHost(ctx *tool.Context, name string) (string, error) {
	hosts, _, _, _, err := readManifest(ctx, false)
	if err != nil {
		return "", err
	}
	host, found := hosts[name]
	if !found {
		return "", fmt.Errorf("host %s not found in manifest", name)
	}
	return host.Location, nil
}

// GerritHost returns the URL that hosts the Gerrit code review system.
func GerritHost(ctx *tool.Context) (string, error) {
	return getHost(ctx, "gerrit")
}

// GitHost returns the URL that hosts the git repositories.
func GitHost(ctx *tool.Context) (string, error) {
	return getHost(ctx, "git")
}

// JiriRoot returns the root of the jiri universe.
func JiriRoot() (string, error) {
	root := os.Getenv(rootEnv)
	if root == "" {
		return "", fmt.Errorf("%v is not set", rootEnv)
	}
	result, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("EvalSymlinks(%v) failed: %v", root, err)
	}
	return result, nil
}
