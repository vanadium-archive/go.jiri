// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"fmt"
	"path/filepath"

	"v.io/jiri/jiri"
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
	// TODO(nlacasse): Tools refer to their project by name, but project name
	// might not be unique.  We really should stop telling telling tools what their
	// projects are.
	project, err := projects.FindUnique(tool.Project)
	if err != nil {
		return "", err
	}
	return filepath.Join(project.Path, tool.Data), nil
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

func toAbs(jirix *jiri.X, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(jirix.Root, path)
}

// toRel returns the given path relative to JIRI_ROOT, if it is not already a
// relative path.
func toRel(jirix *jiri.X, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Rel(jirix.Root, path)
}
