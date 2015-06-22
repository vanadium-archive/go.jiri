// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/set"
)

// parseProjectNames identifies the set of projects that a v23 command
// should be applied to.
func parseProjectNames(ctx *tool.Context, args []string, projects map[string]util.Project, defaultProjects map[string]struct{}) []string {
	names := []string{}
	if len(args) == 0 {
		// Use the default set of projects.
		names = set.String.ToSlice(defaultProjects)
	} else {
		for _, name := range args {
			if _, ok := projects[name]; ok {
				names = append(names, name)
			} else {
				// Issue a warning if the target project does not exist in the
				// project manifest.
				fmt.Fprintf(ctx.Stderr(), "WARNING: project %q does not exist in the project manifest and will be skipped\n", name)
			}
		}
	}
	return names
}
