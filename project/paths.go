// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri/jiri"
)

const (
	rootEnv              = "JIRI_ROOT"
	metadataDirName      = ".jiri"
	metadataFileName     = "metadata.v2"
	metadataProfilesFile = ".jiri_profiles"
)

// DataDirPath returns the path to the data directory of the given tool.
// TODO(nlacasse): DataDirPath is currently broken because we don't set the
// tool.Name variable when building each tool.  Luckily, only the jiri tool has
// uses DataDirPath, and the default tool name is "jiri", so nothing actually
// breaks.  We should revisit the whole data directory thing, and in particular
// see if we can get rid of tools having to know their own names.
func DataDirPath(jirix *jiri.X, toolName string) (string, error) {
	_, projects, tools, _, err := readManifest(jirix, false)
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

func getHost(jirix *jiri.X, name string) (string, error) {
	hosts, _, _, _, err := readManifest(jirix, false)
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
func GerritHost(jirix *jiri.X) (string, error) {
	return getHost(jirix, "gerrit")
}

// GitHost returns the URL that hosts the git repositories.
func GitHost(jirix *jiri.X) (string, error) {
	return getHost(jirix, "git")
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
	if !filepath.IsAbs(result) {
		return "", fmt.Errorf("JIRI_ROOT must be absolute path: %v", rootEnv)
	}
	return filepath.Clean(result), nil
}

// ToAbs returns the given path rooted in JIRI_ROOT, if it is not already an
// absolute path.
func ToAbs(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	root, err := JiriRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, path), nil
}

// ToRel returns the given path relative to JIRI_ROOT, if it is not already a
// relative path.
func ToRel(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return path, nil
	}
	root, err := JiriRoot()
	if err != nil {
		return "", err
	}
	return filepath.Rel(root, path)
}
