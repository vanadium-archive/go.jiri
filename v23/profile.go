// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
)

var (
	defaultDirPerm  = os.FileMode(0755)
	defaultFilePerm = os.FileMode(0644)
	knownProfiles   = map[string]struct{}{
		"arm":         struct{}{},
		"android":     struct{}{},
		"java":        struct{}{},
		"nodejs":      struct{}{},
		"syncbase":    struct{}{},
		"third-party": struct{}{},
		"web":         struct{}{},
	}
)

const (
	// Number of retries for profile setup.
	numRetries              = 3
	actionCompletedFileName = ".vanadium_action_completed"
)

// cmdProfile represents the "v23 profile" command.
var cmdProfile = &cmdline.Command{
	Name:  "profile",
	Short: "Manage vanadium profiles",
	Long: `
To facilitate development across different platforms, vanadium defines
platform-independent profiles that map different platforms to a set
of libraries and tools that can be used for a factor of vanadium
development.
`,
	Children: []*cmdline.Command{cmdProfileList, cmdProfileSetup},
}

// cmdProfileList represents the "v23 profile list" command.
var cmdProfileList = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runProfileList),
	Name:   "list",
	Short:  "List known vanadium profiles",
	Long:   "List known vanadium profiles.",
}

func runProfileList(env *cmdline.Env, _ []string) error {
	profiles := []string{}
	for p := range knownProfiles {
		profiles = append(profiles, p)
	}
	sort.Strings(profiles)
	for _, p := range profiles {
		fmt.Fprintf(env.Stdout, "%s\n", p)
	}
	return nil
}

// cmdProfileSetup represents the "v23 profile setup" command.
var cmdProfileSetup = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runProfileSetup),
	Name:     "setup",
	Short:    "Set up the given vanadium profiles",
	Long:     "Set up the given vanadium profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to set up.",
}

func runProfileSetup(env *cmdline.Env, args []string) error {
	// Check that the profiles to be set up exist.
	for _, arg := range args {
		if _, ok := knownProfiles[arg]; !ok {
			return env.UsageErrorf("profile %v does not exist", arg)
		}
	}

	// Setup the profiles.
	t := true
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &t,
	})
	for _, arg := range args {
		setupFn := func() error {
			var err error
			for i := 1; i <= numRetries; i++ {
				fmt.Fprintf(ctx.Stdout(), fmt.Sprintf("Attempt #%d\n", i))
				if err = setup(ctx, runtime.GOOS, arg); err == nil {
					return nil
				}
				fmt.Fprintf(ctx.Stdout(), "ERROR: %v\n", err)
			}
			return err
		}
		if err := ctx.Run().Function(setupFn, fmt.Sprintf("Set up profile %q", arg)); err != nil {
			return err
		}
	}
	return nil
}

type unknownProfileErr string

func (e unknownProfileErr) Error() string {
	return fmt.Sprintf("unknown profile %q", e)
}

func reportNotImplemented(ctx *tool.Context, os, profile string) {
	ctx.Run().Output([]string{fmt.Sprintf("profile %q is not implemented on %q", profile, os)})
}

func setup(ctx *tool.Context, os, profile string) error {
	switch os {
	case "darwin":
		switch profile {
		case "android":
			return setupAndroidDarwin(ctx)
		case "java":
			return setupJavaDarwin(ctx)
		case "syncbase":
			return setupSyncbaseDarwin(ctx)
		case "third-party":
			return setupThirdPartyDarwin(ctx)
		case "nodejs":
			return setupNodejsDarwin(ctx)
		case "web":
			return setupWebDarwin(ctx)
		default:
			reportNotImplemented(ctx, os, profile)
		}
	case "linux":
		switch profile {
		case "android":
			return setupAndroidLinux(ctx)
		case "arm":
			return setupArmLinux(ctx)
		case "java":
			return setupJavaLinux(ctx)
		case "syncbase":
			return setupSyncbaseLinux(ctx)
		case "nodejs":
			return setupNodejsLinux(ctx)
		case "web":
			return setupWebLinux(ctx)
		default:
			reportNotImplemented(ctx, os, profile)
		}
	default:
		reportNotImplemented(ctx, os, profile)
	}
	return nil
}

func atomicAction(ctx *tool.Context, installFn func() error, dir, message string) error {
	atomicFn := func() error {
		actionCompletedFile := filepath.Join(dir, actionCompletedFileName)
		if dir != "" && directoryExists(ctx, dir) {
			// If the dir exists but the actionCompletedFile doesn't, then it means
			// the previous action didn't finish.
			// Remove the dir so we can perform the action again.
			if !fileExists(ctx, actionCompletedFile) {
				ctx.Run().RemoveAll(dir)
			} else {
				return nil
			}
		}
		if err := installFn(); err != nil {
			if dir != "" {
				ctx.Run().RemoveAll(dir)
			}
			return err
		}
		if err := ctx.Run().WriteFile(actionCompletedFile, []byte("completed"), 0644); err != nil {
			return err
		}
		return nil
	}
	return ctx.Run().Function(atomicFn, message)
}

func directoryExists(ctx *tool.Context, dir string) bool {
	return ctx.Run().Command("test", "-d", dir) == nil
}

func fileExists(ctx *tool.Context, file string) bool {
	return ctx.Run().Command("test", "-f", file) == nil
}

type androidPkg struct {
	name      string
	directory string
}

func installAndroidPkg(ctx *tool.Context, sdkRoot string, pkg androidPkg) error {
	installPkgFn := func() error {
		// Identify all indexes that match the given package.
		var out bytes.Buffer
		androidBin := filepath.Join(sdkRoot, "tools", "android")
		androidArgs := []string{"list", "sdk", "--all", "--no-https"}
		opts := ctx.Run().Opts()
		opts.Stdout = &out
		opts.Stderr = &out
		if err := ctx.Run().CommandWithOpts(opts, androidBin, androidArgs...); err != nil {
			return err
		}
		scanner, indexes := bufio.NewScanner(&out), []int{}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Index(line, pkg.name) != -1 {
				// The output of "android list sdk --all" looks as follows:
				//
				// Packages available for installation or update: 118
				//    1- Android SDK Tools, revision 23.0.5
				//    2- Android SDK Platform-tools, revision 21
				//    3- Android SDK Build-tools, revision 21.1
				// ...
				//
				// The following logic gets the package index.
				index, err := strconv.Atoi(strings.TrimSpace(line[:4]))
				if err != nil {
					return fmt.Errorf("%v", err)
				}
				indexes = append(indexes, index)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("Scan() failed: %v", err)
		}
		switch {
		case len(indexes) == 0:
			return fmt.Errorf("no package matching %q found", pkg.name)
		case len(indexes) > 1:
			return fmt.Errorf("multiple packages matching %q found", pkg.name)
		}

		// Install the target package.
		androidArgs = []string{"update", "sdk", "--no-ui", "--all", "--no-https", "--filter", fmt.Sprintf("%d", indexes[0])}
		var stdin, stdout bytes.Buffer
		stdin.WriteString("y") // pasing "y" to accept Android's license agreement
		opts = ctx.Run().Opts()
		opts.Stdin = &stdin
		opts.Stdout = &stdout
		opts.Stderr = &stdout
		err := ctx.Run().CommandWithOpts(opts, androidBin, androidArgs...)
		if err != nil || verboseFlag {
			fmt.Fprintf(ctx.Stdout(), out.String())
		}
		return err
	}
	return atomicAction(ctx, installPkgFn, pkg.directory, fmt.Sprintf("Install %s", pkg.name))
}

// installDeps identifies the dependencies that need to be installed
// and installs them using the OS-specific package manager.
func installDeps(ctx *tool.Context, pkgs []string) error {
	installDepsFn := func() error {
		switch runtime.GOOS {
		case "linux":
			installPkgs := []string{}
			for _, pkg := range pkgs {
				if err := run(ctx, "dpkg", []string{"-s", pkg}, nil); err != nil {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"apt-get", "install", "-y"}, installPkgs...)
				if err := ctx.Run().Command("sudo", args...); err != nil {
					return err
				}
			}
		case "darwin":
			installPkgs := []string{}
			for _, pkg := range pkgs {
				if err := run(ctx, "brew", []string{"ls", pkg}, nil); err != nil {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"install"}, installPkgs...)
				if err := ctx.Run().Command("brew", args...); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ctx.Run().Function(installDepsFn, "Install dependencies")
}

func run(ctx *tool.Context, bin string, args []string, env map[string]string) error {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	opts.Env = env
	err := ctx.Run().CommandWithOpts(opts, bin, args...)
	if err != nil || verboseFlag {
		fmt.Fprintf(ctx.Stdout(), out.String())
	}
	return err
}

// setupArmLinux sets up the arm profile for linux.
//
// For more on Go cross-compilation for arm/linux information see:
// http://www.bootc.net/archives/2012/05/26/how-to-build-a-cross-compiler-for-your-raspberry-pi/
func setupArmLinux(ctx *tool.Context) (e error) {
	root, err := util.V23Root()
	if err != nil {
		return err
	}

	// Install dependencies.
	pkgs := []string{
		"automake", "bison", "bzip2", "curl", "flex", "g++", "gawk",
		"gettext", "gperf", "libncurses5-dev", "libtool", "subversion", "texinfo",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	// Download and build arm/linux cross-compiler for Go.
	repoDir := filepath.Join(root, "third_party", "repos")
	goDir := filepath.Join(repoDir, "go_arm")
	installGoFn := func() error {
		if err := ctx.Run().MkdirAll(repoDir, defaultDirPerm); err != nil {
			return err
		}
		makeEnv := envvar.VarsFromOS()
		unsetGoEnv(makeEnv)
		makeEnv.Set("GOARCH", "arm")
		makeEnv.Set("GOOS", "linux")
		return installGo14(ctx, goDir, makeEnv)
	}
	if err := atomicAction(ctx, installGoFn, goDir, "Download and build Go for arm/linux"); err != nil {
		return err
	}

	// Build and install crosstool-ng.
	xgccOutDir := filepath.Join(root, "third_party", "cout", "xgcc")
	installNgFn := func() error {
		xgccSrcDir := filepath.Join(root, "third_party", "csrc", "crosstool-ng-1.20.0")
		if err := ctx.Run().Chdir(xgccSrcDir); err != nil {
			return err
		}
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, nil); err != nil {
			return err
		}
		if err := run(ctx, "./configure", []string{fmt.Sprintf("--prefix=%v", xgccOutDir)}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installNgFn, xgccOutDir, "Build and install crosstool-ng"); err != nil {
		return err
	}

	// Build arm/linux gcc tools.
	xgccToolDir := filepath.Join(xgccOutDir, "arm-unknown-linux-gnueabi")
	installXgccFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		if err := ctx.Run().Chdir(tmpDir); err != nil {
			return err
		}
		bin := filepath.Join(xgccOutDir, "bin", "ct-ng")
		if err := run(ctx, bin, []string{"arm-unknown-linux-gnueabi"}, nil); err != nil {
			return err
		}
		dataPath, err := util.DataDirPath(ctx, tool.Name)
		if err != nil {
			return err
		}
		configFile := filepath.Join(dataPath, "crosstool-ng-1.20.0.config")
		config, err := ioutil.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("ReadFile(%v) failed: %v", configFile, err)
		}
		old, new := "/usr/local/vanadium", filepath.Join(root, "third_party", "cout")
		newConfig := strings.Replace(string(config), old, new, -1)
		newConfigFile := filepath.Join(tmpDir, ".config")
		if err := ctx.Run().WriteFile(newConfigFile, []byte(newConfig), defaultFilePerm); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", newConfigFile, err)
		}
		if err := run(ctx, bin, []string{"oldconfig"}, nil); err != nil {
			return err
		}
		if err := run(ctx, bin, []string{"build"}, nil); err != nil {
			return err
		}
		// crosstool-ng build creates the tool output directory with no write
		// permissions. Change it so that atomicAction can create the
		// "action completed" file.
		dirinfo, err := os.Stat(xgccToolDir)
		if err != nil {
			return err
		}
		if err := os.Chmod(xgccToolDir, dirinfo.Mode()|0755); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installXgccFn, xgccToolDir, "Build arm/linux gcc tools"); err != nil {
		ctx.Run().RemoveAll(xgccToolDir)
		return err
	}

	// Create arm/linux gcc symlinks.
	xgccLinkDir := filepath.Join(xgccOutDir, "cross_arm")
	installLinksFn := func() error {
		if err := ctx.Run().MkdirAll(xgccLinkDir, defaultDirPerm); err != nil {
			return err
		}
		if err := ctx.Run().Chdir(xgccLinkDir); err != nil {
			return err
		}
		binDir := filepath.Join(xgccToolDir, "bin")
		fileInfoList, err := ioutil.ReadDir(binDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", binDir, err)
		}
		for _, fileInfo := range fileInfoList {
			prefix := "arm-unknown-linux-gnueabi-"
			if strings.HasPrefix(fileInfo.Name(), prefix) {
				src := filepath.Join(binDir, fileInfo.Name())
				dst := filepath.Join(xgccLinkDir, strings.TrimPrefix(fileInfo.Name(), prefix))
				if err := ctx.Run().Symlink(src, dst); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := atomicAction(ctx, installLinksFn, xgccLinkDir, "Create arm/linux gcc symlinks"); err != nil {
		return err
	}

	return nil
}

// setupAndroidCommon prepares the shared cross-platform parts of the android
// setup.
func setupAndroidCommon(ctx *tool.Context, os string) (e error) {
	root, err := util.V23Root()
	if err != nil {
		return err
	}

	// Install dependencies.
	pkgs := []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	androidRoot := filepath.Join(root, "third_party", "android")

	// Download Android SDK.
	sdkRoot := filepath.Join(androidRoot, "android-sdk-linux")
	installSdkFn := func() error {
		if err := ctx.Run().MkdirAll(sdkRoot, defaultDirPerm); err != nil {
			return err
		}
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		remote := "https://dl.google.com/android/android-sdk_r23-linux.tgz"
		local := filepath.Join(tmpDir, "android-sdk.tgz")
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := run(ctx, "tar", []string{"-C", androidRoot, "-xzf", local}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installSdkFn, sdkRoot, "Download Android SDK"); err != nil {
		return err
	}

	// Install Android SDK packagess.
	androidPkgs := []androidPkg{
		androidPkg{"Android SDK Platform-tools, revision 22", filepath.Join(sdkRoot, "platform-tools")},
		androidPkg{"SDK Platform Android 4.4.2, API 19, revision 4", filepath.Join(sdkRoot, "platforms", "android-19")},
		androidPkg{"Android SDK Build-tools, revision 21.0.2", filepath.Join(sdkRoot, "build-tools")},
		androidPkg{"ARM EABI v7a System Image, Android API 19, revision 2", filepath.Join(sdkRoot, "system-images", "android-19")},
	}
	for _, pkg := range androidPkgs {
		if err := installAndroidPkg(ctx, sdkRoot, pkg); err != nil {
			return err
		}
	}

	// Update Android SDK tools.
	toolPkg := androidPkg{"Android SDK Tools", ""}
	if err := installAndroidPkg(ctx, sdkRoot, toolPkg); err != nil {
		return err
	}

	// Download Android NDK.
	ndkRoot := filepath.Join(androidRoot, "ndk-toolchain")
	installNdkFn := func() error {
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		filename := "android-ndk-r9d-" + os + "-x86_64.tar.bz2"
		remote := "https://dl.google.com/android/ndk/" + filename
		local := filepath.Join(tmpDir, filename)
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		if err := run(ctx, "tar", []string{"-C", tmpDir, "-xjf", local}, nil); err != nil {
			return err
		}
		ndkBin := filepath.Join(tmpDir, "android-ndk-r9d", "build", "tools", "make-standalone-toolchain.sh")
		ndkArgs := []string{ndkBin, "--platform=android-9", "--install-dir=" + ndkRoot}
		if err := run(ctx, "bash", ndkArgs, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installNdkFn, ndkRoot, "Download Android NDK"); err != nil {
		return err
	}

	// Install Android Go.
	androidGo := filepath.Join(androidRoot, "go")
	installGoFn := func() error {
		makeEnv := envvar.VarsFromOS()
		unsetGoEnv(makeEnv)
		makeEnv.Set("CGO_ENABLED", "1")
		makeEnv.Set("CC_FOR_TARGET", filepath.Join(ndkRoot, "bin", "arm-linux-androideabi-gcc"))
		makeEnv.Set("GOOS", "android")
		makeEnv.Set("GOARCH", "arm")
		makeEnv.Set("GOARM", "7")
		return installGo15(ctx, androidGo, makeEnv)
	}
	return atomicAction(ctx, installGoFn, androidGo, "Download and build Android Go")
}

// setupAndroidDarwin sets up the android profile for darwin.
func setupAndroidDarwin(ctx *tool.Context) error {
	return setupAndroidCommon(ctx, "darwin")
}

// setupAndroidLinux sets up the android profile for linux.
func setupAndroidLinux(ctx *tool.Context) error {
	return setupAndroidCommon(ctx, "linux")
}

// setupJavaCommon contains cross-platform actions to setup the java profile.
func setupJavaCommon(ctx *tool.Context) error {
	root, err := util.V23Root()
	if err != nil {
		return err
	}
	javaRoot := filepath.Join(root, "third_party", "java")
	javaGo := filepath.Join(javaRoot, "go")
	installGoFn := func() error {
		return installGo15(ctx, javaGo, envvar.VarsFromOS())
	}
	return atomicAction(ctx, installGoFn, javaGo, "Download and build Java Go")
}

// setupJavaDarwin sets up the java profile for mac.
func setupJavaDarwin(ctx *tool.Context) error {
	// Prompt the user to install JDK 1.7+, if not already installed.
	// (Note that JDK cannot be installed via Homebrew.)
	javaHomeBin := "/usr/libexec/java_home"
	if err := run(ctx, javaHomeBin, []string{"-t", "CommandLine", "-v", "1.7+"}, nil); err != nil {
		run(ctx, javaHomeBin, []string{"-t", "CommandLine", "--request"}, nil)
		// Wait for JDK to be installed.
		fmt.Println("Please follow the OS X prompt instructions to install JDK 1.7+.")
		for true {
			time.Sleep(5 * time.Second)
			if err = run(ctx, javaHomeBin, []string{"-t", "CommandLine", "-v", "1.7+"}, nil); err == nil {
				break
			}
		}
	}
	if err := setupJavaCommon(ctx); err != nil {
		return err
	}
	if os.Getenv("JAVA_HOME") == "" {
		fmt.Println("Please set JAVA_HOME environment variable to the root of your JDK directory.")
	}
	return nil
}

// setupJavaLinux sets up the java profile for linux.
func setupJavaLinux(ctx *tool.Context) error {
	return setupJavaCommon(ctx)
}

// setupThirdPartyDarwin sets up the third-party profile for darwin.
func setupThirdPartyDarwin(ctx *tool.Context) error {
	if err := run(ctx, "brew", []string{"tap", "homebrew/dupes"}, nil); err != nil {
		return err
	}
	{
		var out bytes.Buffer
		opts := ctx.Run().Opts()
		opts.Stdout = io.MultiWriter(&out, opts.Stdout)
		opts.Stderr = io.MultiWriter(&out, opts.Stdout)
		if err := ctx.Run().CommandWithOpts(opts, "brew", "install", "openssh", "--with-brewed-openssl", "--with-keychain-support"); err != nil {
			return err
		}
	}
	{
		var out bytes.Buffer
		opts := ctx.Run().Opts()
		opts.Stdout = io.MultiWriter(&out, opts.Stdout)
		opts.Stderr = io.MultiWriter(&out, opts.Stdout)
		if err := ctx.Run().CommandWithOpts(opts, "brew", "install", "dbus"); err != nil {
			return err
		}
	}
	return nil
}

// setupNodejsDarwin sets up the nodejs profile for darwin.
func setupNodejsDarwin(ctx *tool.Context) error {
	return setupNodejsCommon(ctx)
}

// setupNodejsLinux sets up the nodejs profile for linux.
func setupNodejsLinux(ctx *tool.Context) error {
	// Install dependencies.
	pkgs := []string{"g++"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	return setupNodejsCommon(ctx)
}

// setupNodejsCommon sets up the nodejs profile.
func setupNodejsCommon(ctx *tool.Context) error {
	root, err := util.V23Root()
	if err != nil {
		return err
	}

	// Build and install NodeJS.
	nodeOutDir := filepath.Join(root, "third_party", "cout", "node")
	installNodeFn := func() error {
		nodeSrcDir := filepath.Join(root, "third_party", "csrc", "node-v0.10.24")
		if err := ctx.Run().Chdir(nodeSrcDir); err != nil {
			return err
		}
		if err := run(ctx, "./configure", []string{fmt.Sprintf("--prefix=%v", nodeOutDir)}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installNodeFn, nodeOutDir, "Build and install NodeJS"); err != nil {
		return err
	}

	return nil
}

// setupWebDarwin sets up the web profile for darwin.
func setupWebDarwin(ctx *tool.Context) error {
	return setupWebCommon(ctx)
}

// setupWebLinux sets up the web profile for linux.
func setupWebLinux(ctx *tool.Context) error {
	// Install dependencies.
	pkgs := []string{"g++", "libc6-i386", "zip"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	return setupWebCommon(ctx)
}

// setupWebCommon sets up the web profile.
func setupWebCommon(ctx *tool.Context) error {
	root, err := util.V23Root()
	if err != nil {
		return err
	}

	// Build and install NodeJS.
	if err := setupNodejsCommon(ctx); err != nil {
		return err
	}

	// Clone the Go Ppapi compiler.
	repoDir := filepath.Join(root, "third_party", "repos")
	goPpapiRepoDir := filepath.Join(repoDir, "go_ppapi")
	remote, revision := "https://vanadium.googlesource.com/release.go.ppapi", "5e967194049bd1a6f097854f09fcbbbaa21afc05"
	cloneGoPpapiFn := func() error {
		if err := ctx.Run().MkdirAll(repoDir, defaultDirPerm); err != nil {
			return err
		}
		if err := gitCloneRepo(ctx, remote, revision, goPpapiRepoDir); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, cloneGoPpapiFn, goPpapiRepoDir, "Clone Go Ppapi repository"); err != nil {
		return err
	}

	// Compile the Go Ppapi compiler.
	goPpapiBinDir := filepath.Join(goPpapiRepoDir, "bin")
	compileGoPpapiFn := func() error {
		goPpapiCompileScript := filepath.Join(goPpapiRepoDir, "src", "make-nacl-amd64p32.sh")
		if err := run(ctx, goPpapiCompileScript, []string{}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, compileGoPpapiFn, goPpapiBinDir, "Compile Go Ppapi compiler"); err != nil {
		return err
	}

	return nil
}

// setupSyncbaseDarwin sets up the syncbase profile for darwin.
func setupSyncbaseDarwin(ctx *tool.Context) error {
	// Install dependencies.
	pkgs := []string{
		"autoconf", "automake", "libtool", "pkg-config",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}
	return setupSyncbaseCommon(ctx)
}

// setupSyncbaseLinux sets up the syncbase profile for linux.
func setupSyncbaseLinux(ctx *tool.Context) error {
	// Install dependencies.
	pkgs := []string{
		"autoconf", "automake", "g++", "g++-multilib", "gcc-multilib", "libtool", "pkg-config",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}
	return setupSyncbaseCommon(ctx)
}

// setupSyncbaseCommon sets up the syncbase profile.
func setupSyncbaseCommon(ctx *tool.Context) (e error) {
	root, err := util.V23Root()
	if err != nil {
		return err
	}

	// TODO(rogulenko): get these var from a config file.
	goos, goarch := runtime.GOOS, os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	var outPrefix string
	if outPrefix, err = util.ThirdPartyCCodePath(goos, goarch); err != nil {
		return err
	}
	// Build and install Snappy.
	snappyOutDir := filepath.Join(outPrefix, "snappy")
	installSnappyFn := func() error {
		snappySrcDir := filepath.Join(root, "third_party", "csrc", "snappy-1.1.2")
		if err := ctx.Run().Chdir(snappySrcDir); err != nil {
			return err
		}
		if err := run(ctx, "autoreconf", []string{"--install", "--force", "--verbose"}, nil); err != nil {
			return err
		}
		args := []string{
			fmt.Sprintf("--prefix=%v", snappyOutDir),
			"--enable-shared=false",
		}
		env := map[string]string{}
		if goarch == "386" {
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		}
		if err := run(ctx, "./configure", args, env); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{fmt.Sprintf("-j%d", runtime.NumCPU())}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"install"}, nil); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"distclean"}, nil); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installSnappyFn, snappyOutDir, "Build and install Snappy"); err != nil {
		return err
	}

	// Build and install LevelDB.
	leveldbOutDir := filepath.Join(outPrefix, "leveldb")
	installLeveldbFn := func() error {
		leveldbSrcDir := filepath.Join(root, "third_party", "csrc", "leveldb")
		if err := ctx.Run().Chdir(leveldbSrcDir); err != nil {
			return err
		}
		if err := run(ctx, "mkdir", []string{"-p", leveldbOutDir}, nil); err != nil {
			return err
		}
		leveldbIncludeDir := filepath.Join(leveldbOutDir, "include")
		if err := run(ctx, "cp", []string{"-R", "include", leveldbIncludeDir}, nil); err != nil {
			return err
		}
		leveldbLibDir := filepath.Join(leveldbOutDir, "lib")
		if err := run(ctx, "mkdir", []string{leveldbLibDir}, nil); err != nil {
			return err
		}
		env := map[string]string{
			"PREFIX":   leveldbLibDir,
			"CXXFLAGS": "-I" + filepath.Join(snappyOutDir, "include"),
			"LDFLAGS":  "-L" + filepath.Join(snappyOutDir, "lib"),
		}
		if goarch == "386" {
			env["CC"] = "gcc -m32"
			env["CXX"] = "g++ -m32"
		}
		if err := run(ctx, "make", []string{"clean"}, env); err != nil {
			return err
		}
		if err := run(ctx, "make", []string{"static"}, env); err != nil {
			return err
		}
		return nil
	}
	if err := atomicAction(ctx, installLeveldbFn, leveldbOutDir, "Build and install LevelDB"); err != nil {
		return err
	}

	return nil
}

// installGo14 installs Go 1.4 at a given location, using the provided
// environment during compilation.
func installGo14(ctx *tool.Context, goDir string, env *envvar.Vars) error {
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	name := "go1.4.2.src.tar.gz"
	remote, local := "https://storage.googleapis.com/golang/"+name, filepath.Join(tmpDir, name)
	if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
		return err
	}
	if err := run(ctx, "tar", []string{"-C", tmpDir, "-xzf", local}, nil); err != nil {
		return err
	}
	if err := ctx.Run().RemoveAll(local); err != nil {
		return err
	}
	if err := ctx.Run().Rename(filepath.Join(tmpDir, "go"), goDir); err != nil {
		return err
	}
	goSrcDir := filepath.Join(goDir, "src")
	if err := ctx.Run().Chdir(goSrcDir); err != nil {
		return err
	}
	makeBin := filepath.Join(goSrcDir, "make.bash")
	makeArgs := []string{"--no-clean"}
	if err := run(ctx, makeBin, makeArgs, env.ToMap()); err != nil {
		return err
	}
	return nil
}

// installGo15 installs Go 1.5 at a given location, using the provided
// environment during compilation.
func installGo15(ctx *tool.Context, goDir string, env *envvar.Vars) error {
	// First install bootstrap Go 1.4 for the host.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	goBootstrapDir := filepath.Join(tmpDir, "go")
	if err := installGo14(ctx, goBootstrapDir, envvar.VarsFromOS()); err != nil {
		return err
	}

	// Install Go 1.5.
	if tmpDir, err = ctx.Run().TempDir("", ""); err != nil {
		return err
	}
	remote, revision := "https://github.com/golang/go.git", "22e4b8167f14bdd33738cfdc21c3396b2341f8fd"
	if err := gitCloneRepo(ctx, remote, revision, tmpDir); err != nil {
		return err
	}
	if err := ctx.Run().Rename(tmpDir, goDir); err != nil {
		return err
	}
	goSrcDir := filepath.Join(goDir, "src")
	if err := ctx.Run().Chdir(goSrcDir); err != nil {
		return err
	}
	makeBin := filepath.Join(goSrcDir, "make.bash")
	env.Set("GOROOT_BOOTSTRAP", goBootstrapDir)
	if err := run(ctx, makeBin, nil, env.ToMap()); err != nil {
		return err
	}
	return nil
}

func unsetGoEnv(env *envvar.Vars) {
	env.Set("CGO_ENABLED", "")
	env.Set("CGO_CFLAGS", "")
	env.Set("CGO_CXXFLAGS", "")
	env.Set("CGO_LDFLAGS", "")
	env.Set("GOARCH", "")
	env.Set("GOBIN", "")
	env.Set("GOOS", "")
	env.Set("GOPATH", "")
	env.Set("GOROOT", "")
}

// gitCloneRepo clones a repo at a specific revision in outDir.
func gitCloneRepo(ctx *tool.Context, remote, revision, outDir string) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)

	if err := ctx.Run().MkdirAll(outDir, defaultDirPerm); err != nil {
		return err
	}
	if err := run(ctx, "git", []string{"clone", remote, outDir}, nil); err != nil {
		return err
	}
	if err := ctx.Run().Chdir(outDir); err != nil {
		return err
	}
	if err := run(ctx, "git", []string{"reset", "--hard", revision}, nil); err != nil {
		return err
	}
	return nil
}
