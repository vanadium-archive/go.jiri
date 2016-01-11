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
	projects, tools, _, err := readManifest(jirix)
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
