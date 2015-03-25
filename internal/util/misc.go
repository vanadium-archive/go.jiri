// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"v.io/x/lib/envutil"
)

// ThirdPartyBinPath returns the path to the given third-party tool
// taking into account the host and the target Go architecture.
func ThirdPartyBinPath(root, name string) (string, error) {
	machineArch, err := envutil.Arch()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(root, "third_party", "go", "bin", name)
	goArch := os.Getenv("GOARCH")
	if goArch != "" && goArch != machineArch {
		bin = filepath.Join(root, "third_party", "go", "bin", fmt.Sprintf("%s_%s", runtime.GOOS, goArch), name)
	}
	return bin, nil
}
