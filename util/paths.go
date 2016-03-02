// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"v.io/jiri"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/host"
)

// ConfigFilePath returns the path to the tools configuration file.
func ConfigFilePath(jirix *jiri.X) (string, error) {
	dataDir, err := project.DataDirPath(jirix, tool.Name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "config.v1.xml"), nil
}

// OncallRotationPath returns the path to the oncall rotation file.
func OncallRotationPath(jirix *jiri.X) (string, error) {
	dataDir, err := project.DataDirPath(jirix, tool.Name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "oncall.v1.xml"), nil
}

// ThirdPartyBinPath returns the path to the given third-party tool
// taking into account the host and the target Go architecture.
func ThirdPartyBinPath(jirix *jiri.X, name string) (string, error) {
	bin := filepath.Join(jirix.Root, "third_party", "go", "bin", name)
	goArch := os.Getenv("GOARCH")
	machineArch, err := host.Arch()
	if err != nil {
		return "", err
	}
	if goArch != "" && goArch != machineArch {
		bin = filepath.Join(jirix.Root, "third_party", "go", "bin", fmt.Sprintf("%s_%s", runtime.GOOS, goArch), name)
	}
	return bin, nil
}
