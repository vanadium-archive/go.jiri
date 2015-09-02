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

	"v.io/jiri/lib/project"
	"v.io/jiri/lib/tool"
	"v.io/x/lib/envvar"
)

const (
	// TODO(nlacasse): Rename to JIRI_PROFILE.
	jiriProfileEnv = "V23_PROFILE"
	javaEnv        = "JAVA_HOME"
)

// TODO(nlacasse): Rename this.
// VanadiumEnvironment returns the environment variables setting for the
// project. The util package captures the original state of the relevant
// environment variables when the tool is initialized and every invocation of
// this function updates this original state according to the jiri tool
// configuration.
//
// By default, the Go and VDL workspaces are added to the GOPATH and VDLPATH
// environment variables respectively. In addition, the V23_PROFILE environment
// variable can be used to activate an environment variable setting for various
// development profiles of the project (e.g. arm, android, java, or nacl).
// Unlike the default setting, the setting enabled by the V23_PROFILE
// environment variable can override existing environment.
func VanadiumEnvironment(ctx *tool.Context) (*envvar.Vars, error) {
	env := envvar.VarsFromOS()
	root, err := project.V23Root()
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
	if err := setSyncbaseEnv(ctx, env, root); err != nil {
		return nil, err
	}
	if profile := os.Getenv(jiriProfileEnv); profile != "" {
		fmt.Fprintf(ctx.Stdout(), `NOTE: Enabling environment variable setting for %q.
This can override values of existing environment variables.
`, profile)
		switch profile {
		case "android":
			// Cross-compilation for android on linux.
			if err := setAndroidEnv(ctx, env, root); err != nil {
				return nil, err
			}
		case "arm":
			// Cross-compilation for arm on linux.
			if err := setArmEnv(ctx, env, root); err != nil {
				return nil, err
			}
		case "java":
			// Building of a Go shared library for Java.
			if err := setJavaEnv(ctx, env, root); err != nil {
				return nil, err
			}
		case "nacl":
			// Cross-compilation for nacl.
			if err := setNaclEnv(ctx, env, root); err != nil {
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
func setJavaEnv(ctx *tool.Context, env *envvar.Vars, root string) error {
	jdkHome := os.Getenv(javaEnv)
	if jdkHome == "" {
		return nil
	}

	// Compile using Java Go 1.5 (installed via 'jiri profile install java').
	javaGoDir := filepath.Join(root, "third_party", "java", "go")

	// Set Go-specific environment variables.
	cflags := env.GetTokens("CGO_CFLAGS", " ")
	cflags = append(cflags, filepath.Join("-I"+jdkHome, "include"), filepath.Join("-I"+jdkHome, "include", runtime.GOOS))
	env.SetTokens("CGO_CFLAGS", cflags, " ")
	env.Set("GOROOT", javaGoDir)

	// Update PATH.
	if _, err := ctx.Run().Stat(javaGoDir); err != nil {
		return fmt.Errorf("Couldn't find java go installation directory %s: did you run \"jiri profile install java\"?", javaGoDir)
	}
	path := env.GetTokens("PATH", ":")
	path = append([]string{filepath.Join(javaGoDir, "bin")}, path...)
	env.SetTokens("PATH", path, ":")
	return nil
}

// setAndroidEnv sets the environment variables used for android
// cross-compilation.
func setAndroidEnv(ctx *tool.Context, env *envvar.Vars, root string) error {
	// Compile using Android Go 1.5 (installed via
	// 'jiri profile install android').
	androidGoDir := filepath.Join(root, "third_party", "android", "go")

	// Set Go-specific environment variables.
	env.Set("GOARCH", "arm")
	env.Set("GOARM", "7")
	env.Set("GOOS", "android")
	env.Set("GOROOT", androidGoDir)

	// Update PATH.
	if _, err := ctx.Run().Stat(androidGoDir); err != nil {
		return fmt.Errorf("Couldn't find android Go installation directory %s: did you run \"jiri profile install android\"?", androidGoDir)
	}
	path := env.GetTokens("PATH", ":")
	path = append([]string{filepath.Join(androidGoDir, "bin")}, path...)
	env.SetTokens("PATH", path, ":")
	return nil
}

// setArmEnv sets the environment variables used for arm cross-compilation.
func setArmEnv(ctx *tool.Context, env *envvar.Vars, root string) error {
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

// setGoPath adds the paths of Go workspaces to the GOPATH variable.
func setGoPath(ctx *tool.Context, env *envvar.Vars, root string, config *Config) error {
	return setPathHelper(ctx, env, "GOPATH", root, config.GoWorkspaces(), "")
}

// setVdlPath adds the paths of VDL workspaces to the VDLPATH variable.
func setVdlPath(ctx *tool.Context, env *envvar.Vars, root string, config *Config) error {
	return setPathHelper(ctx, env, "VDLPATH", root, config.VDLWorkspaces(), "src")
}

// setPathHelper is a utility function for setting path environment
// variables for different types of workspaces.
func setPathHelper(ctx *tool.Context, env *envvar.Vars, name, root string, workspaces []string, suffix string) error {
	path := env.GetTokens(name, ":")
	projects, _, err := project.ReadManifest(ctx)
	if err != nil {
		return err
	}
	for _, workspace := range workspaces {
		absWorkspace := filepath.Join(root, workspace, suffix)
		// Only append an entry to the path if the workspace is rooted
		// under a jiri project that exists locally or vice versa.
		for _, project := range projects {
			// We check if <project.Path> is a prefix of <absWorkspace> to
			// account for Go workspaces nested under a single jiri project,
			// such as: $V23_ROOT/release/projects/chat/go.
			//
			// We check if <absWorkspace> is a prefix of <project.Path> to
			// account for Go workspaces that span multiple jiri projects,
			// such as: $V23_ROOT/release/go.
			if strings.HasPrefix(absWorkspace, project.Path) || strings.HasPrefix(project.Path, absWorkspace) {
				if _, err := ctx.Run().Stat(filepath.Join(absWorkspace)); err == nil {
					path = append(path, absWorkspace)
					break
				}
			}
		}
	}
	env.SetTokens(name, path, ":")
	return nil
}

// TODO(nlacasse): Move setNaclEnv and setSyncbaseEnv out of jiri since they
// are not general-purpose.

// setNaclEnv sets the environment variables used for nacl
// cross-compilation.
func setNaclEnv(ctx *tool.Context, env *envvar.Vars, root string) error {
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
func setSyncbaseEnv(ctx *tool.Context, env *envvar.Vars, root string) error {
	libs := []string{
		"leveldb",
		"snappy",
	}
	// TODO(rogulenko): get these vars from a config file.
	goos, goarch := os.Getenv("GOOS"), os.Getenv("GOARCH")
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	for _, lib := range libs {
		cflags := env.GetTokens("CGO_CFLAGS", " ")
		cxxflags := env.GetTokens("CGO_CXXFLAGS", " ")
		ldflags := env.GetTokens("CGO_LDFLAGS", " ")
		dir, err := ThirdPartyCCodePath(goos, goarch)
		if err != nil {
			return err
		}
		dir = filepath.Join(dir, lib)
		if _, err := ctx.Run().Stat(dir); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			continue
		}
		cflags = append(cflags, filepath.Join("-I"+dir, "include"))
		cxxflags = append(cxxflags, filepath.Join("-I"+dir, "include"))
		ldflags = append(ldflags, filepath.Join("-L"+dir, "lib"))
		if runtime.GOARCH == "linux" {
			ldflags = append(ldflags, "-Wl,-rpath", filepath.Join(dir, "lib"))
		}
		env.SetTokens("CGO_CFLAGS", cflags, " ")
		env.SetTokens("CGO_CXXFLAGS", cxxflags, " ")
		env.SetTokens("CGO_LDFLAGS", ldflags, " ")
	}
	return nil
}
