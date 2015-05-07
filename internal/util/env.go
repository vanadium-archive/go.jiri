// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/x/devtools/internal/envutil"
	"v.io/x/devtools/internal/tool"
)

const (
	v23ProfileEnv = "V23_PROFILE"
	javaEnv       = "JDK_HOME"
)

// VanadiumEnvironment returns the environment variables setting for
// the Vanadium project. The util package captures the original state
// of the relevant environment variables when the tool is initialized
// and every invocation of this function updates this original state
// according to the v23 tool configuration.
//
// By default, the Vanadium Go and VDL workspaces are added to the
// GOPATH and VDLPATH environment variables respectively. In addition,
// the V23_PROFILE environment variable can be used to activate an
// environment variable setting for various development profiles of
// the Vanadium project (e.g. arm, android, java, or nacl). Unlike the
// default setting, the setting enabled by the V23_PROFILE environment
// variable can override existing environment.
func VanadiumEnvironment(ctx *tool.Context) (*envutil.Snapshot, error) {
	env := envutil.NewSnapshotFromOS()
	root, err := V23Root()
	if err != nil {
		return nil, err
	}
	config, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	env.Set("CGO_ENABLED", "1")
	if err := setGoPath(ctx, env, root, config); err != nil {
		return nil, err
	}
	if err := setVdlPath(ctx, env, root, config); err != nil {
		return nil, err
	}
	if err := setSyncbaseEnv(env, root); err != nil {
		return nil, err
	}
	if profile := os.Getenv(v23ProfileEnv); profile != "" {
		fmt.Fprintf(ctx.Stdout(), `NOTE: Enabling environment variable setting for %q.
This can override values of existing environment variables.
`, profile)
		switch profile {
		case "android":
			// Cross-compilation for android on linux.
			if err := setAndroidEnv(env, root); err != nil {
				return nil, err
			}
		case "arm":
			// Cross-compilation for arm on linux.
			if err := setArmEnv(env, root); err != nil {
				return nil, err
			}
		case "java":
			// Building of a Go shared library for Java.
			if err := setJavaEnv(env); err != nil {
				return nil, err
			}
		case "nacl":
			// Cross-compilation for nacl.
			if err := setNaclEnv(env, root); err != nil {
				return nil, err
			}
		default:
			fmt.Fprintf(ctx.Stderr(), "Unknown environment profile %q", profile)
		}
	}
	return env, nil
}

// setJavaEnv sets the environment variables used for building a Go
// shared library that is invoked from Java code. If Java is not
// installed on the host, this function is a no-op.
func setJavaEnv(env *envutil.Snapshot) error {
	jdkHome := os.Getenv(javaEnv)
	if jdkHome == "" {
		return nil
	}
	cflags := env.GetTokens("CGO_CFLAGS", " ")
	cflags = append(cflags, filepath.Join("-I"+jdkHome, "include"), filepath.Join("-I"+jdkHome, "include", "linux"))
	env.SetTokens("CGO_CFLAGS", cflags, " ")
	return nil
}

// setAndroidEnv sets the environment variables used for android
// cross-compilation.
func setAndroidEnv(env *envutil.Snapshot, root string) error {
	// Set the environment variables needed for building Go shared
	// libraries for Java.
	if err := setJavaEnv(env); err != nil {
		return err
	}

	// Set Go specific environment variables.
	env.Set("GOARCH", "arm")
	env.Set("GOARM", "7")
	env.Set("GOOS", "android")

	// Add the path to android cross-compiler to the PATH.
	path := env.GetTokens("PATH", ":")
	path = append([]string{
		filepath.Join(root, "third_party", "android", "go", "bin"),
	}, path...)
	env.SetTokens("PATH", path, ":")

	return nil
}

// setArmEnv sets the environment variables used for android
// cross-compilation.
func setArmEnv(env *envutil.Snapshot, root string) error {
	// Set Go specific environment variables.
	env.Set("GOARCH", "arm")
	env.Set("GOARM", "6")
	env.Set("GOOS", "linux")

	// Add the paths to arm cross-compilation tools to the PATH.
	path := env.GetTokens("PATH", ":")
	path = append([]string{
		filepath.Join(root, "third_party", "cout", "xgcc", "cross_arm"),
		filepath.Join(root, "third_party", "repos", "go_arm", "bin"),
	}, path...)
	env.SetTokens("PATH", path, ":")

	return nil
}

// setGoPath adds the paths to Vanadium Go workspaces to the GOPATH
// variable.
func setGoPath(ctx *tool.Context, env *envutil.Snapshot, root string, config *Config) error {
	return setPathHelper(ctx, env, "GOPATH", root, config.GoWorkspaces())
}

// setVdlPath adds the paths to Vanadium VDL workspaces to the VDLPATH
// variable.
func setVdlPath(ctx *tool.Context, env *envutil.Snapshot, root string, config *Config) error {
	return setPathHelper(ctx, env, "VDLPATH", root, config.VDLWorkspaces())
}

// setPathHelper is a utility function for setting path environment
// variables for different types of workspaces.
func setPathHelper(ctx *tool.Context, env *envutil.Snapshot, name, root string, workspaces []string) error {
	path := env.GetTokens(name, ":")
	projects, _, err := readManifest(ctx, false)
	if err != nil {
		return err
	}
	for _, workspace := range workspaces {
		absWorkspace := filepath.Join(root, workspace)
		// Only append an entry to the path if the workspace is rooted
		// under a v23 project that exists locally or vice versa.
		for _, project := range projects {
			// We check if <project.Path> is a prefix of <absWorkspace> to
			// account for Go workspaces nested under a single v23 project,
			// such as: $V23_ROOT/release/projects/chat/go.
			//
			// We check if <absWorkspace> is a prefix of <project.Path> to
			// account for Go workspaces that span multiple v23 projects,
			// such as: $V23_ROOT/release/go.
			if strings.HasPrefix(absWorkspace, project.Path) || strings.HasPrefix(project.Path, absWorkspace) {
				if _, err := os.Stat(filepath.Join(absWorkspace)); err == nil {
					path = append(path, absWorkspace)
					break
				}
			}
		}
	}
	env.SetTokens(name, path, ":")
	return nil
}

// setNaclEnv sets the environment variables used for nacl
// cross-compilation.
func setNaclEnv(env *envutil.Snapshot, root string) error {
	env.Set("GOARCH", "amd64p32")
	env.Set("GOOS", "nacl")

	// Add the path to nacl cross-compiler to the PATH.
	path := env.GetTokens("PATH", ":")
	path = append([]string{
		filepath.Join(root, "third_party", "repos", "go_ppapi", "bin"),
	}, path...)
	env.SetTokens("PATH", path, ":")

	return nil
}

// setSyncbaseEnv adds the LevelDB third-party C++ libraries Vanadium
// Go code depends on to the CGO_CFLAGS and CGO_LDFLAGS variables.
func setSyncbaseEnv(env *envutil.Snapshot, root string) error {
	cflags := env.GetTokens("CGO_CFLAGS", " ")
	cxxflags := env.GetTokens("CGO_CXXFLAGS", " ")
	ldflags := env.GetTokens("CGO_LDFLAGS", " ")
	dir := filepath.Join(root, "third_party", "cout", "leveldb")
	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Stat(%v) failed: %v", dir, err)
		}
		return nil
	}
	cflags = append(cflags, filepath.Join("-I"+dir, "include"))
	cxxflags = append(cxxflags, filepath.Join("-I"+dir, "include"))
	ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"))
	// TODO(jsimsa): Currently, the "v23 profile setup syncbase" command
	// only compiles LevelDB C++ libraries for the host architecture. As
	// a consequence, the syncbase component can only be compiled if the
	// target platform matches the host platform.
	if runtime.GOARCH == "linux" {
		ldflags = append(ldflags, "-Wl,-rpath", filepath.Join(dir, "lib"))
	}
	env.SetTokens("CGO_CFLAGS", cflags, " ")
	env.SetTokens("CGO_CXXFLAGS", cxxflags, " ")
	env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	return nil
}
