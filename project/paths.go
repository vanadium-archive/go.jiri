// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
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
	return filepath.Join(jirix.Root, "release", "go", "src", "v.io", "x", "devtools", "data"), nil
}
