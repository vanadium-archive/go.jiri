// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// IMPORTANT: When modifying existing profiles or creating new ones,
// keep the following in mind.
//
// If an existing profile installation logic changes, the version
// recorded in the <knownProfiles> variable should be incremented, the
// old uninstall logic should be kept and trigger for instances of the
// previous version, and new uninstall logic should be introduced.
//
// If new profile is introduced, make sure it is added to
// <knownProfiles> and implementation of both the install and
// uninstall logic is provided.

package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/project"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
)

type profilesSchema struct {
	XMLName  xml.Name      `xml:"profiles"`
	Profiles []profileInfo `xml:"profile"`
}

type profileInfos []profileInfo

func (pis profileInfos) Len() int           { return len(pis) }
func (pis profileInfos) Less(i, j int) bool { return pis[i].Name < pis[j].Name }
func (pis profileInfos) Swap(i, j int)      { pis[i], pis[j] = pis[j], pis[i] }

type profileTarget struct {
	Arch    string `xml:"arch,attr"`
	OS      string `xml:"os,attr"`
	Version int    `xml:"version,attr"`
}

func (pt profileTarget) String() string {
	return fmt.Sprintf("arch:%v os:%v", pt.Arch, pt.OS)
}

func (pt profileTarget) Equals(pt2 profileTarget) bool {
	return pt.Arch == pt2.Arch && pt.OS == pt2.OS
}

type profileInfo struct {
	Name    string          `xml:"name,attr"`
	Targets []profileTarget `xml:"target"`
	XMLName xml.Name        `xml:"profile"`
	version int
}

var (
	defaultDirPerm  = os.FileMode(0755)
	defaultFilePerm = os.FileMode(0644)
	knownProfiles   = map[string]profileInfo{
		"arm": profileInfo{
			Name:    "arm",
			version: 1,
		},
		"android": profileInfo{
			Name:    "android",
			version: 2,
		},
		"java": profileInfo{
			Name:    "java",
			version: 1,
		},
		"nacl": profileInfo{
			Name:    "nacl",
			version: 1,
		},
		"nodejs": profileInfo{
			Name:    "nodejs",
			version: 1,
		},
		"syncbase": profileInfo{
			Name:    "syncbase",
			version: 1,
		},
	}
)

const (
	// Number of retries for profile installation.
	numRetries              = 3
	actionCompletedFileName = ".vanadium_action_completed"
)

// cmdProfile represents the "v23 profile" command.
var cmdProfile = &cmdline.Command{
	Name:  "profile",
	Short: "Manage vanadium profiles",
	Long: `
To facilitate development across different host platforms, vanadium
defines platform-independent "profiles" that map different platforms
to a set of libraries and tools that can be used for a facet of
vanadium development.

Each profile can be in one of three states: absent, up-to-date, or
out-of-date. The subcommands of the profile command realize the
following transitions:

  install:   absent => up-to-date
  update:    out-of-date => up-to-date
  uninstall: up-to-date or out-of-date => absent

In addition, a profile can transition from being up-to-date to
out-of-date by the virtue of a new version of the profile being
released.

To enable cross-compilation, a profile can be installed for multiple
targets. If a profile supports multiple targets the above state
transitions are applied on a profile + target basis.
`,
	Children: []*cmdline.Command{
		cmdProfileInstall,
		cmdProfileList,
		cmdProfileSetup,
		cmdProfileUninstall,
		cmdProfileUpdate,
	},
}

// addTarget adds the given target to the given profile.
func addTarget(profiles map[string]profileInfo, target profileTarget, name string) {
	profile, ok := profiles[name]
	if !ok {
		profile = knownProfiles[name]
	}
	profile.Targets = append(profile.Targets, target)
	profiles[name] = profile
}

// getTarget returns the target environment for the profile command.
func getTarget() profileTarget {
	target := profileTarget{
		Arch: os.Getenv("GOARCH"),
		OS:   os.Getenv("GOOS"),
	}
	if target.Arch == "" {
		target.Arch = runtime.GOARCH
	}
	if target.OS == "" {
		target.OS = runtime.GOOS
	}
	return target
}

// removeTarget removes the given target from the given profile.
func removeTarget(profiles map[string]profileInfo, target profileTarget, name string) {
	profile, ok := profiles[name]
	if !ok {
		return
	}
	for i, t := range profile.Targets {
		if target.Equals(t) {
			profile.Targets = append(profile.Targets[:i], profile.Targets[i+1:]...)
			if len(profile.Targets) == 0 {
				delete(profiles, name)
			} else {
				profiles[name] = profile
			}
			return
		}
	}
}

// lookupVersion returns the version for the given profile target
// combination in the given collection of profiles.
func lookupVersion(profiles map[string]profileInfo, target profileTarget, name string) int {
	if profile, ok := profiles[name]; ok {
		for _, t := range profile.Targets {
			if target.Equals(t) {
				return t.Version
			}
		}
	}
	return -1
}

// readV23Profiles reads information about installed v23 profiles into
// memory.
func readV23Profiles(ctx *tool.Context) (map[string]profileInfo, error) {
	file, err := project.V23ProfilesFile()
	if err != nil {
		return nil, err
	}
	data, err := ctx.Run().ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]profileInfo{}, nil
		}
		return nil, err
	}
	var schema profilesSchema
	if err := xml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	profiles := profileInfos(schema.Profiles)
	sort.Sort(profiles)
	result := map[string]profileInfo{}
	for _, profile := range profiles {
		result[profile.Name] = profile
	}
	return result, nil
}

// writeV23Profiles writes information about installed v23 profiles to
// disk.
func writeV23Profiles(ctx *tool.Context, profiles map[string]profileInfo) error {
	var schema profilesSchema
	for _, profile := range profiles {
		schema.Profiles = append(schema.Profiles, profile)
	}
	sort.Sort(profileInfos(schema.Profiles))
	data, err := xml.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}
	file, err := project.V23ProfilesFile()
	if err != nil {
		return err
	}
	if err := ctx.Run().WriteFile(file, data, defaultFileMode); err != nil {
		return err
	}
	return nil
}

// cmdProfileInstall represents the "v23 profile install" command.
var cmdProfileInstall = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runProfileInstall),
	Name:     "install",
	Short:    "Install the given vanadium profiles",
	Long:     "Install the given vanadium profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to install.",
}

func runProfileInstall(env *cmdline.Env, args []string) error {
	// Check that the profiles to be installed exist.
	for _, arg := range args {
		if _, ok := knownProfiles[arg]; !ok {
			return env.UsageErrorf("profile %q does not exist", arg)
		}
	}

	// Create contexts.
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	t := true
	verboseCtx := ctx.Clone(tool.ContextOpts{Verbose: &t})

	// Find out what profiles are installed and what the operation
	// target is.
	profiles, err := readV23Profiles(ctx)
	if err != nil {
		return err
	}
	target := getTarget()

	// Install the profiles.
	for _, arg := range args {
		// Check if the profile is installed for the given target.
		if version := lookupVersion(profiles, target, arg); version != -1 {
			fmt.Fprintf(ctx.Stdout(), "NOTE: profile %q is already installed for target %q\n", arg, target)
			continue
		}
		// Install the profile for the given target.
		installFn := func() error {
			var err error
			for i := 1; i <= numRetries; i++ {
				fmt.Fprintf(ctx.Stdout(), fmt.Sprintf("Attempt #%d\n", i))
				if err = install(verboseCtx, target, arg); err == nil {
					return nil
				}
				fmt.Fprintf(ctx.Stdout(), "ERROR: %v\n", err)
			}
			return err
		}
		if err := verboseCtx.Run().Function(installFn, fmt.Sprintf("Install profile %q", arg)); err != nil {
			return err
		}
		// Persist the information about the profile installation.
		target.Version = knownProfiles[arg].version
		addTarget(profiles, target, arg)
		if err := writeV23Profiles(ctx, profiles); err != nil {
			return err
		}
	}
	return nil
}

func reportNotImplemented(ctx *tool.Context, profile string, target profileTarget) {
	ctx.Run().Output([]string{fmt.Sprintf("profile %q is not implemented for target %q", profile, target)})
}

func install(ctx *tool.Context, target profileTarget, profile string) error {
	switch target.OS {
	case "darwin":
		switch profile {
		case "android":
			return installAndroidDarwin(ctx, target)
		case "java":
			return installJavaDarwin(ctx, target)
		case "nacl":
			return installNaclDarwin(ctx, target)
		case "nodejs":
			return installNodeJSDarwin(ctx, target)
		case "syncbase":
			return installSyncbaseDarwin(ctx, target)
		default:
			reportNotImplemented(ctx, profile, target)
		}
	case "linux":
		switch profile {
		case "android":
			return installAndroidLinux(ctx, target)
		case "arm":
			return installArmLinux(ctx, target)
		case "java":
			return installJavaLinux(ctx, target)
		case "nacl":
			return installNaclLinux(ctx, target)
		case "nodejs":
			return installNodeJSLinux(ctx, target)
		case "syncbase":
			return installSyncbaseLinux(ctx, target)
		default:
			reportNotImplemented(ctx, profile, target)
		}
	default:
		reportNotImplemented(ctx, profile, target)
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

// installArmLinux installs the arm profile for linux.
//
// For more on Go cross-compilation for arm/linux information see:
// http://www.bootc.net/archives/2012/05/26/how-to-build-a-cross-compiler-for-your-raspberry-pi/
func installArmLinux(ctx *tool.Context, target profileTarget) (e error) {
	root, err := project.V23Root()
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
		dataPath, err := project.DataDirPath(ctx, tool.Name)
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
		dirinfo, err := ctx.Run().Stat(xgccToolDir)
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

// installAndroidCommon prepares the shared cross-platform parts of the android setup.
func installAndroidCommon(ctx *tool.Context, target profileTarget) (e error) {
	root, err := project.V23Root()
	if err != nil {
		return err
	}

	// Install dependencies.
	var pkgs []string
	switch target.OS {
	case "linux":
		pkgs = []string{"ant", "autoconf", "bzip2", "default-jdk", "gawk", "lib32z1", "lib32stdc++6"}
	case "darwin":
		pkgs = []string{"ant", "autoconf", "gawk"}
	default:
		return fmt.Errorf("unsupported OS: %s", target.OS)
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	androidRoot := filepath.Join(root, "third_party", "android")
	var sdkRoot string
	switch target.OS {
	case "linux":
		sdkRoot = filepath.Join(androidRoot, "android-sdk-linux")
	case "darwin":
		sdkRoot = filepath.Join(androidRoot, "android-sdk-macosx")
	default:
		return fmt.Errorf("unsupported OS: %s", target.OS)
	}

	// Download Android SDK.
	installSdkFn := func() error {
		if err := ctx.Run().MkdirAll(androidRoot, defaultDirPerm); err != nil {
			return err
		}
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		var filename string
		switch target.OS {
		case "linux":
			filename = "android-sdk_r23-linux.tgz"
		case "darwin":
			filename = "android-sdk_r23-macosx.zip"
		default:
			return fmt.Errorf("unsupported OS: %s", target.OS)
		}
		remote, local := "https://dl.google.com/android/"+filename, filepath.Join(tmpDir, filename)
		if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
			return err
		}
		switch target.OS {
		case "linux":
			if err := run(ctx, "tar", []string{"-C", androidRoot, "-xzf", local}, nil); err != nil {
				return err
			}
		case "darwin":
			if err := run(ctx, "unzip", []string{"-d", androidRoot, local}, nil); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported OS: %s", target.OS)
		}
		return nil
	}
	if err := atomicAction(ctx, installSdkFn, sdkRoot, "Download Android SDK"); err != nil {
		return err
	}

	// Install Android SDK packagess.
	androidPkgs := []androidPkg{
		androidPkg{"Android SDK Platform-tools", filepath.Join(sdkRoot, "platform-tools")},
		androidPkg{"SDK Platform Android 4.4.2, API 19, revision 4", filepath.Join(sdkRoot, "platforms", "android-19")},
		androidPkg{"Android SDK Build-tools, revision 21.0.2", filepath.Join(sdkRoot, "build-tools")},
		androidPkg{"ARM EABI v7a System Image, Android API 19, revision 3", filepath.Join(sdkRoot, "system-images", "android-19")},
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
			return fmt.Errorf("TempDir() failed: %v", err)
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		filename := "android-ndk-r9d-" + target.OS + "-x86_64.tar.bz2"
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
		return installGo15(ctx, androidGo, nil, makeEnv)
	}
	return atomicAction(ctx, installGoFn, androidGo, "Download and build Android Go")
}

// installAndroidDarwin installs the android profile for Darwin.
func installAndroidDarwin(ctx *tool.Context, target profileTarget) error {
	return installAndroidCommon(ctx, target)
}

// installAndroidLinux installs the android profile for Linux.
func installAndroidLinux(ctx *tool.Context, target profileTarget) error {
	return installAndroidCommon(ctx, target)
}

// installGradle downloads and unzips Gradle into the given directory.
func installGradle(ctx *tool.Context, targetDir string) (e error) {
	if err := ctx.Run().MkdirAll(targetDir, 0755); err != nil {
		return err
	}
	zipFilename := "gradle-2.5-bin.zip"
	remote, local := "https://services.gradle.org/distributions/"+zipFilename, filepath.Join(targetDir, zipFilename)
	if err := run(ctx, "curl", []string{"-Lo", local, remote}, nil); err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(local) }, &e)
	if err := run(ctx, "unzip", []string{"-d", targetDir, local}, nil); err != nil {
		return err
	}
	return ctx.Run().Symlink(filepath.Join(targetDir, "gradle-2.5", "bin", "gradle"), filepath.Join(targetDir, "gradle"))
}

// installJavaCommon contains cross-platform actions to install the
// java profile.
func installJavaCommon(ctx *tool.Context, target profileTarget) error {
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	javaRoot := filepath.Join(root, "third_party", "java")
	javaGo := filepath.Join(javaRoot, "go")
	gradleDir := filepath.Join(javaRoot, "gradle")
	installGoFn := func() error {
		return installGo15(ctx, javaGo, nil, envvar.VarsFromOS())
	}
	installGradleFn := func() error {
		return installGradle(ctx, gradleDir)
	}
	if err := atomicAction(ctx, installGoFn, javaGo, "Download and build Java Go"); err != nil {
		return err
	}
	return atomicAction(ctx, installGradleFn, gradleDir, "Download and unzip Gradle")
}

// hasJDK returns true iff the JDK already exists on the machine and
// is correctly set up.
func hasJDK(ctx *tool.Context) bool {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" {
		return false
	}
	_, err := ctx.Run().Stat(filepath.Join(javaHome, "include", "jni.h"))
	return err == nil
}

// installJavaDarwin installs the java profile for darwin.
func installJavaDarwin(ctx *tool.Context, target profileTarget) error {
	if !hasJDK(ctx) {
		// Prompt the user to install JDK 1.7+, if not already installed.
		// (Note that JDK cannot be installed via Homebrew.)
		javaHomeBin := "/usr/libexec/java_home"
		if err := run(ctx, javaHomeBin, []string{"-t", "CommandLine", "-v", "1.7+"}, nil); err != nil {
			fmt.Printf("Couldn't find a valid JDK installation under JAVA_HOME (%s): installing a new JDK.\n", os.Getenv("JAVA_HOME"))
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
	}
	if err := installJavaCommon(ctx, target); err != nil {
		return err
	}
	return nil
}

// installJavaLinux installs the java profile for linux.
func installJavaLinux(ctx *tool.Context, target profileTarget) error {
	if !hasJDK(ctx) {
		fmt.Printf("Couldn't find a valid JDK installation under JAVA_HOME (%s): installing a new JDK.\n", os.Getenv("JAVA_HOME"))
		if err := installDeps(ctx, []string{"openjdk-7-jdk"}); err != nil {
			return err
		}
	}
	if err := installJavaCommon(ctx, target); err != nil {
		return err
	}
	return nil
}

// installNodeJSDarwin installs the nodeJS profile for darwin.
func installNodeJSDarwin(ctx *tool.Context, target profileTarget) error {
	return installNodeJSCommon(ctx, target)
}

// installNodeJSLinux installs the nodejs profile for linux.
func installNodeJSLinux(ctx *tool.Context, target profileTarget) error {
	// Install dependencies.
	pkgs := []string{"g++"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	return installNodeJSCommon(ctx, target)
}

// installNodeJSCommon installs the nodejs profile.
func installNodeJSCommon(ctx *tool.Context, target profileTarget) error {
	root, err := project.V23Root()
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
	if err := atomicAction(ctx, installNodeFn, nodeOutDir, "Build and install node.js"); err != nil {
		return err
	}

	return nil
}

// installNaclDarwin installs the nacl profile for darwin.
func installNaclDarwin(ctx *tool.Context, target profileTarget) error {
	return installNaclCommon(ctx, target)
}

// installNaclLinux installs the nacl profile for linux.
func installNaclLinux(ctx *tool.Context, target profileTarget) error {
	// Install dependencies.
	pkgs := []string{"g++", "libc6-i386", "zip"}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}

	return installNaclCommon(ctx, target)
}

// installNaclCommon installs the nacl profile.
func installNaclCommon(ctx *tool.Context, target profileTarget) error {
	root, err := project.V23Root()
	if err != nil {
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

// installSyncbaseDarwin installs the syncbase profile for darwin.
func installSyncbaseDarwin(ctx *tool.Context, target profileTarget) error {
	// Install dependencies.
	pkgs := []string{
		"autoconf", "automake", "libtool", "pkg-config",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}
	return installSyncbaseCommon(ctx, target)
}

// installSyncbaseLinux installs the syncbase profile for linux.
func installSyncbaseLinux(ctx *tool.Context, target profileTarget) error {
	// Install dependencies.
	pkgs := []string{
		"autoconf", "automake", "g++", "g++-multilib", "gcc-multilib", "libtool", "pkg-config",
	}
	if err := installDeps(ctx, pkgs); err != nil {
		return err
	}
	return installSyncbaseCommon(ctx, target)
}

// installSyncbaseCommon installs the syncbase profile.
func installSyncbaseCommon(ctx *tool.Context, target profileTarget) (e error) {
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	outPrefix, err := util.ThirdPartyCCodePath(target.OS, target.Arch)
	if err != nil {
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
		env := map[string]string{
			// NOTE(nlacasse): The -fPIC flag is needed to compile Syncbase Mojo service.
			"CXXFLAGS": " -fPIC",
		}
		if target.Arch == "386" {
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
			"PREFIX": leveldbLibDir,
			// NOTE(nlacasse): The -fPIC flag is needed to compile Syncbase Mojo service.
			"CXXFLAGS": "-I" + filepath.Join(snappyOutDir, "include") + " -fPIC",
			"LDFLAGS":  "-L" + filepath.Join(snappyOutDir, "lib"),
		}
		if target.Arch == "386" {
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
func installGo15(ctx *tool.Context, goDir string, patchFiles []string, env *envvar.Vars) error {
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
	remote, revision := "https://github.com/golang/go.git", "cc6554f750ccaf63bcdcc478b2a60d71ca76d342"
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
	// Apply patches, if any.
	for _, patchFile := range patchFiles {
		if err := run(ctx, "git", []string{"apply", patchFile}, nil); err != nil {
			return err
		}
	}
	makeBin := filepath.Join(goSrcDir, "make.bash")
	env.Set("GOROOT_BOOTSTRAP", goBootstrapDir)
	if err := run(ctx, makeBin, nil, env.ToMap()); err != nil {
		return err
	}
	return nil
}

// unsetGoEnv unsets Go environment variables in the given
// environment.
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

// cmdProfileList represents the "v23 profile list" command.
//
// TODO(jsimsa): Add support for listing installed profiles.
var cmdProfileList = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runProfileList),
	Name:   "list",
	Short:  "List known vanadium profiles",
	Long:   "List known vanadium profiles.",
}

func runProfileList(env *cmdline.Env, _ []string) error {
	var profiles []string
	for profile := range knownProfiles {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	for _, p := range profiles {
		fmt.Fprintf(env.Stdout, "%s\n", p)
	}
	return nil
}

// cmdProfileSetup represents the "v23 profile setup" command. This
// command is identical to "v23 profile install" and is provided for
// backwards compatibility.
var cmdProfileSetup = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runProfileInstall),
	Name:     "setup",
	Short:    "Set up the given vanadium profiles",
	Long:     "Set up the given vanadium profiles. This command is identical to 'install' and is provided for backwards compatibility.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to set up.",
}

// cmdProfileUninstall represents the "v23 profile uninstall" command.
var cmdProfileUninstall = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runProfileUninstall),
	Name:     "uninstall",
	Short:    "Uninstall the given vanadium profiles",
	Long:     "Uninstall the given vanadium profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to uninstall.",
}

func runProfileUninstall(env *cmdline.Env, args []string) error {
	// Check that the profiles to be uninstalled exist.
	for _, arg := range args {
		if _, ok := knownProfiles[arg]; !ok {
			return env.UsageErrorf("profile %v does not exist", arg)
		}
	}

	// Create contexts.
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	t := true
	verboseCtx := ctx.Clone(tool.ContextOpts{Verbose: &t})

	// Find out what profiles are installed and what the operation
	// target is.
	profiles, err := readV23Profiles(ctx)
	if err != nil {
		return err
	}
	target := getTarget()

	// Uninstall the profiles.
	for _, arg := range args {
		// Check if the profile is installed for the given target.
		version := lookupVersion(profiles, target, arg)
		if version == -1 {
			fmt.Fprintf(ctx.Stdout(), "NOTE: profile %q is not installed for target %q\n", arg, target)
			continue
		}
		// Uninstall the profile for the given target.
		uninstallFn := func() error {
			return uninstall(verboseCtx, target, arg, version)
		}
		if err := verboseCtx.Run().Function(uninstallFn, fmt.Sprintf("Uninstall profile %q", arg)); err != nil {
			return err
		}
		// Persist the information about the profile installation.
		removeTarget(profiles, target, arg)
		if err := writeV23Profiles(ctx, profiles); err != nil {
			return err
		}
	}
	return nil
}

func uninstall(ctx *tool.Context, target profileTarget, profile string, version int) error {
	switch target.OS {
	case "darwin":
		switch profile {
		case "android":
			return uninstallAndroidDarwin(ctx, target, version)
		case "java":
			return uninstallJavaDarwin(ctx, target, version)
		case "nacl":
			return uninstallNaclDarwin(ctx, target, version)
		case "nodejs":
			return uninstallNodeJSDarwin(ctx, target, version)
		case "syncbase":
			return uninstallSyncbaseDarwin(ctx, target, version)
		default:
			reportNotImplemented(ctx, profile, target)
		}
	case "linux":
		switch profile {
		case "android":
			return uninstallAndroidLinux(ctx, target, version)
		case "arm":
			return uninstallArmLinux(ctx, target, version)
		case "java":
			return uninstallJavaLinux(ctx, target, version)
		case "nacl":
			return uninstallNaclLinux(ctx, target, version)
		case "nodejs":
			return uninstallNodeJSLinux(ctx, target, version)
		case "syncbase":
			return uninstallSyncbaseLinux(ctx, target, version)
		default:
			reportNotImplemented(ctx, profile, target)
		}
	default:
		reportNotImplemented(ctx, profile, target)
	}
	return nil
}

// uninstallAndroidDarwin uninstalls the android profile for darwin.
func uninstallAndroidDarwin(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallAndroidCommon(ctx, target, version)
}

// uninstallAndroidLinux uninstalls the android profile for linux.
func uninstallAndroidLinux(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallAndroidCommon(ctx, target, version)
}

// uninstallAndroidCommon uninstalls the android profile.
func uninstallAndroidCommon(ctx *tool.Context, target profileTarget, version int) error {
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	androidRoot := filepath.Join(root, "third_party", "android")
	return ctx.Run().RemoveAll(androidRoot)
}

// uninstallArmLinux uninstalls the arm profile for linux.
func uninstallArmLinux(ctx *tool.Context, target profileTarget, version int) error {
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	goRepoDir := filepath.Join(root, "third_party", "repos", "go_arm")
	if err := ctx.Run().RemoveAll(goRepoDir); err != nil {
		return err
	}
	xgccOutDir := filepath.Join(root, "third_party", "cout", "xgcc")
	if err := ctx.Run().RemoveAll(xgccOutDir); err != nil {
		return err
	}
	return nil
}

// uninstallJavaDarwin uninstalls the java profile for darwin.
func uninstallJavaDarwin(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallJavaCommon(ctx, target, version)
}

// uninstallJavaLinux uninstalls the java profile for linux.
func uninstallJavaLinux(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallJavaCommon(ctx, target, version)
}

// uninstallJavaCommon uninstalls the java profile. Currently this means
// uninstalling Gradle, in the future we will take care of removing any JDK
// installed by `v23 profile install java`.
func uninstallJavaCommon(ctx *tool.Context, target profileTarget, version int) error {
	// TODO(spetrovic): Implement JDK removal.
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	gradleRoot := filepath.Join(root, "third_party", "java", "gradle")
	return ctx.Run().RemoveAll(gradleRoot)
}

// uninstallNaclDarwin uninstalls the nacl profile for darwin.
func uninstallNaclDarwin(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallNaclCommon(ctx, target, version)
}

// uninstallNaclLinux uninstalls the nacl profile for linux.
func uninstallNaclLinux(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallNaclCommon(ctx, target, version)
}

// uninstallNaclCommon uninstalls the nacl profile.
func uninstallNaclCommon(ctx *tool.Context, target profileTarget, version int) error {
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	goPpapiRepoDir := filepath.Join(root, "third_party", "repos", "go_ppapi")
	if err := ctx.Run().RemoveAll(goPpapiRepoDir); err != nil {
		return err
	}
	return nil
}

// uninstallNodeJSDarwin uninstalls the nodejs profile for darwin.
func uninstallNodeJSDarwin(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallNodeJSCommon(ctx, target, version)
}

// uninstallNodeJSLinux uninstalls the nodejs profile for linux.
func uninstallNodeJSLinux(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallNodeJSCommon(ctx, target, version)
}

// uninstallNodeJSCommon uninstalls the nodejs profile.
func uninstallNodeJSCommon(ctx *tool.Context, target profileTarget, version int) error {
	root, err := project.V23Root()
	if err != nil {
		return err
	}
	nodeOutDir := filepath.Join(root, "third_party", "cout", "node")
	if err := ctx.Run().RemoveAll(nodeOutDir); err != nil {
		return err
	}
	return nil
}

// uninstallSyncbaseDarwin uninstalls the syncbase profile for darwin.
func uninstallSyncbaseDarwin(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallSyncbaseCommon(ctx, target, version)
}

// uninstallSyncbaseLinux uninstalls the syncbase profile for linux.
func uninstallSyncbaseLinux(ctx *tool.Context, target profileTarget, version int) error {
	return uninstallSyncbaseCommon(ctx, target, version)
}

// uninstallSyncbaseCommon uninstalls the syncbase profile.
func uninstallSyncbaseCommon(ctx *tool.Context, target profileTarget, version int) error {
	outPrefix, err := util.ThirdPartyCCodePath(target.OS, target.Arch)
	if err != nil {
		return err
	}
	snappyOutDir := filepath.Join(outPrefix, "snappy")
	if err := ctx.Run().RemoveAll(snappyOutDir); err != nil {
		return err
	}
	leveldbOutDir := filepath.Join(outPrefix, "leveldb")
	if err := ctx.Run().RemoveAll(leveldbOutDir); err != nil {
		return err
	}
	return nil
}

// cmdProfileUpdate represents the "v23 profile update" command.
var cmdProfileUpdate = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runProfileUpdate),
	Name:     "update",
	Short:    "Update the given vanadium profiles",
	Long:     "Update the given vanadium profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to update.",
}

func runProfileUpdate(env *cmdline.Env, args []string) error {
	// Check that the profiles to be updated exist.
	for _, arg := range args {
		if _, ok := knownProfiles[arg]; !ok {
			return env.UsageErrorf("profile %v does not exist", arg)
		}
	}

	// Create contexts.
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	t := true
	verboseCtx := ctx.Clone(tool.ContextOpts{Verbose: &t})

	// Find out what profiles are installed and what the operation
	// target is.
	profiles, err := readV23Profiles(ctx)
	if err != nil {
		return err
	}
	target := getTarget()

	// Update the profiles.
	for _, arg := range args {
		version := lookupVersion(profiles, target, arg)
		// Check if the profile is installed for the given target.
		if version == -1 {
			fmt.Fprintf(ctx.Stdout(), "NOTE: profile %q is not installed for target %q\n", arg, target)
			continue
		}
		// Check if the profile installation for the given target is up-to-date.
		if knownProfiles[arg].version == version {
			fmt.Fprintf(ctx.Stdout(), "NOTE: profile %q is already up-to-date\n", arg)
			continue
		}
		// Uninstall the old version.
		uninstallFn := func() error {
			return uninstall(verboseCtx, target, arg, version)
		}
		if err := verboseCtx.Run().Function(uninstallFn, fmt.Sprintf("Uninstall old version of profile %q", arg)); err != nil {
			return err
		}
		// Persist the information about the profile installation.
		removeTarget(profiles, target, arg)
		if err := writeV23Profiles(ctx, profiles); err != nil {
			return err
		}
		// Install the new version.
		installFn := func() error {
			var err error
			for i := 1; i <= numRetries; i++ {
				fmt.Fprintf(ctx.Stdout(), fmt.Sprintf("Attempt #%d\n", i))
				if err = install(verboseCtx, target, arg); err == nil {
					return nil
				}
				fmt.Fprintf(ctx.Stdout(), "ERROR: %v\n", err)
			}
			return err
		}
		if err := verboseCtx.Run().Function(installFn, fmt.Sprintf("Install new version of profile %q", arg)); err != nil {
			return err
		}
		// Persist the information about the profile installation.
		target.Version = knownProfiles[arg].version
		addTarget(profiles, target, arg)
		if err := writeV23Profiles(ctx, profiles); err != nil {
			return err
		}
	}
	return nil
}
