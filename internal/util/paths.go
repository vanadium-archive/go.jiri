// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"v.io/jiri/internal/project"
	"v.io/jiri/internal/tool"
	"v.io/x/lib/host"
)

// ConfigFilePath returns the path to the tools configuration file.
func ConfigFilePath(ctx *tool.Context) (string, error) {
	dataDir, err := project.DataDirPath(ctx, tool.Name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "config.v1.xml"), nil
}

// OncallRotationPath returns the path to the oncall rotation file.
func OncallRotationPath(ctx *tool.Context) (string, error) {
	dataDir, err := project.DataDirPath(ctx, tool.Name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "oncall.v1.xml"), nil
}

// ThirdPartyBinPath returns the path to the given third-party tool
// taking into account the host and the target Go architecture.
func ThirdPartyBinPath(name string) (string, error) {
	root, err := project.V23Root()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(root, "third_party", "go", "bin", name)
	goArch := os.Getenv("GOARCH")
	machineArch, err := host.Arch()
	if err != nil {
		return "", err
	}
	if goArch != "" && goArch != machineArch {
		bin = filepath.Join(root, "third_party", "go", "bin", fmt.Sprintf("%s_%s", runtime.GOOS, goArch), name)
	}
	return bin, nil
}

// ThirdPartyCCodePath returns that path to the directory containing built
// binaries for the target OS and architecture.
func ThirdPartyCCodePath(os, arch string) (string, error) {
	root, err := project.V23Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "third_party", "cout", fmt.Sprintf("%s_%s", os, arch)), nil
}
