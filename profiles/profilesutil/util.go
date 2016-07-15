// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package profilesutil provides utility routines for implementing profiles.
package profilesutil

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"v.io/jiri"
	"v.io/jiri/tool"
)

const (
	DefaultDirPerm  = os.FileMode(0755)
	DefaultFilePerm = os.FileMode(0644)
)

// IsFNLHost returns true iff the host machine is running FNL
// TODO(bprosnitz) We should find a better way to detect that the machine is
// running FNL
// TODO(bprosnitz) This is needed in part because fnl is not currently a
// GOHOSTOS. This should probably be handled by having hosts that are separate
// from GOHOSTOSs similarly to how targets are defined.
func IsFNLHost() bool {
	return os.Getenv("FNL_SYSTEM") != ""
}

var (
	usingAptitude = false
	usingYum      = false
	usingPacman   = false

	testAptitudeOnce, testYumOnce, testPacmanOnce sync.Once
)

// UsingAptitude returns true if the aptitude package manager (debian, ubuntu)
// is being used by the underlying OS.
func UsingAptitude(jirix *jiri.X) bool {
	testAptitudeOnce.Do(func() {
		usingAptitude = jirix.NewSeq().Last("apt-get", "-v") == nil
	})
	return usingAptitude
}

// UsingYum returns true if the yum/rpm package manager (redhat) is being used
// by the underlying OS.
func UsingYum(jirix *jiri.X) bool {
	testYumOnce.Do(func() {
		usingYum = jirix.NewSeq().Last("yum", "--version") == nil
	})
	return usingYum
}

// UsingPacman returns true if the pacman package manager (archlinux) is being
// used by the underlying OS.
func UsingPacman(jirix *jiri.X) bool {
	testPacmanOnce.Do(func() {
		usingPacman = jirix.NewSeq().Last("pacman", "-V") == nil
	})
	return usingPacman
}

// AtomicAction performs an action 'atomically' by keeping track of successfully
// completed actions in the supplied completion log and re-running them if they
// are not successfully logged therein after deleting the entire contents of the
// dir parameter. Consequently it does not make sense to apply AtomicAction to
// the same directory in sequence.
func AtomicAction(jirix *jiri.X, installFn func() error, dir, message string) error {
	atomicFn := func() error {
		completionLogPath := filepath.Join(dir, ".complete")
		s := jirix.NewSeq()
		if dir != "" {
			if exists, _ := s.IsDir(dir); exists {
				// If the dir exists but the completionLogPath doesn't, then it
				// means the previous action didn't finish.
				// Remove the dir so we can perform the action again.
				if exists, _ := s.IsFile(completionLogPath); !exists {
					s.RemoveAll(dir).Done()
				} else {
					if jirix.Verbose() {
						fmt.Fprintf(jirix.Stdout(), "AtomicAction: %s already completed in %s\n", message, dir)
					}
					return nil
				}
			}
		}
		if err := installFn(); err != nil {
			if dir != "" {
				s.RemoveAll(dir).Done()
			}
			return err
		}
		return s.WriteFile(completionLogPath, []byte("completed"), DefaultFilePerm).Done()
	}
	return jirix.NewSeq().Call(atomicFn, message).Done()
}

func brewList(jirix *jiri.X) (map[string]bool, error) {
	var out bytes.Buffer
	err := jirix.NewSeq().Capture(&out, &out).Last("brew", "list")
	if err != nil || tool.VerboseFlag {
		fmt.Fprintf(jirix.Stdout(), "%s", out.String())
	}
	scanner := bufio.NewScanner(&out)
	pkgs := map[string]bool{}
	for scanner.Scan() {
		pkgs[scanner.Text()] = true
	}
	return pkgs, err
}

func linuxList(jirix *jiri.X, pkgs []string) (map[string]bool, error) {
	aptitude, yum, pacman := UsingAptitude(jirix), UsingYum(jirix), UsingPacman(jirix)
	cmd := ""
	opt := ""
	switch {
	case aptitude:
		cmd = "dpkg"
		opt = "-L"
	case yum:
		cmd = "yum"
		opt = "list"
	case pacman:
		cmd = "pacman"
		opt = "-Q"
	default:
		return nil, fmt.Errorf("no usable package manager found, tested for aptitude, yum and pacman")
	}
	s := jirix.NewSeq()
	installedPkgs := map[string]bool{}
	for _, pkg := range pkgs {
		if err := s.Capture(ioutil.Discard, ioutil.Discard).Last(cmd, opt, pkg); err == nil {
			installedPkgs[pkg] = true
		}
	}
	return installedPkgs, nil
}

func linuxInstall(jirix *jiri.X, pkgs []string) []string {
	aptitude, yum, pacman := UsingAptitude(jirix), UsingYum(jirix), UsingPacman(jirix)
	var cmd []string
	switch {
	case aptitude:
		cmd = append(cmd, "apt-get", "install", "-y")
	case yum:
		cmd = append(cmd, "yum", "install", "-y")
	case pacman:
		cmd = append(cmd, "pacman", "-S", "--noconfirm")
	default:
		fmt.Fprintf(jirix.Stdout(), "no usable package manager found, tested for aptitude, yum and pacman")
		return nil
	}
	return append(cmd, pkgs...)
}

// MissingOSPackages returns the subset of the supplied packages that are
// missing from the underlying operating system and hence will need to
// be installed.
func MissingOSPackages(jirix *jiri.X, pkgs []string) ([]string, error) {
	installedPkgs := map[string]bool{}
	switch runtime.GOOS {
	case "linux":
		if IsFNLHost() {
			fmt.Fprintf(jirix.Stdout(), "skipping %v on FNL host\n", pkgs)
			break
		}
		var err error
		installedPkgs, err = linuxList(jirix, pkgs)
		if err != nil {
			return nil, err
		}
	case "darwin":
		var err error
		installedPkgs, err = brewList(jirix)
		if err != nil {
			return nil, err
		}
	}
	missing := []string{}
	for _, pkg := range pkgs {
		if !installedPkgs[pkg] {
			missing = append(missing, pkg)
		}
	}
	return missing, nil
}

// OSPackagesInstallCommands returns the list of commands required to
// install the specified packages on the underlying operating system.
func OSPackageInstallCommands(jirix *jiri.X, pkgs []string) [][]string {
	cmds := make([][]string, 0, 1)
	switch runtime.GOOS {
	case "linux":
		if IsFNLHost() {
			fmt.Fprintf(jirix.Stdout(), "skipping %v on FNL host\n", pkgs)
			break
		}
		if len(pkgs) > 0 {
			cmds = append(cmds, linuxInstall(jirix, pkgs))
		}
	case "darwin":
		if len(pkgs) > 0 {
			return append(cmds, append([]string{"brew", "install"}, pkgs...))
		}
	}
	return cmds
}

// Fetch downloads the specified url and saves it to dst.
func Fetch(jirix *jiri.X, dst, url string) error {
	s := jirix.NewSeq()
	s.Output([]string{"fetching " + url})
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got non-200 status code while getting %v: %v", url, resp.StatusCode)
	}
	file, err := s.Create(dst)
	if err != nil {
		return err
	}
	if _, err := s.Copy(file, resp.Body); err != nil {
		return err
	}
	return file.Close()
}

// Untar untars the file in srcFile and puts resulting files in directory dstDir.
func Untar(jirix *jiri.X, srcFile, dstDir string) error {
	s := jirix.NewSeq()
	if err := s.MkdirAll(dstDir, 0755).Done(); err != nil {
		return err
	}
	return s.Output([]string{"untarring " + srcFile + " into " + dstDir}).
		Pushd(dstDir).
		Last("tar", "xvf", srcFile)
}

// Unzip unzips the file in srcFile and puts resulting files in directory dstDir.
func Unzip(jirix *jiri.X, srcFile, dstDir string) error {
	r, err := zip.OpenReader(srcFile)
	if err != nil {
		return err
	}
	defer r.Close()

	unzipFn := func(zFile *zip.File) error {
		rc, err := zFile.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		s := jirix.NewSeq()
		fileDst := filepath.Join(dstDir, zFile.Name)
		if zFile.FileInfo().IsDir() {
			return s.MkdirAll(fileDst, zFile.Mode()).Done()
		}

		// Make sure the parent directory exists.  Note that sometimes files
		// can appear in a zip file before their directory.
		dirmode := zFile.Mode() | 0100
		if dirmode&0060 != 0 {
			// "group" has read or write permissions, so give
			// execute permissions on the directory.
			dirmode = dirmode | 0010
		}
		if err := s.MkdirAll(filepath.Dir(fileDst), dirmode).Done(); err != nil {
			return err
		}
		file, err := s.OpenFile(fileDst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zFile.Mode())
		if err != nil {

			return err
		}
		defer file.Close()
		_, err = s.Copy(file, rc)
		return err
	}
	s := jirix.NewSeq()
	s.Output([]string{"unzipping " + srcFile})
	for _, zFile := range r.File {
		s.Output([]string{"extracting " + zFile.Name})
		s.Call(func() error { return unzipFn(zFile) }, "unzipFn(%s)", zFile.Name)
	}
	return s.Done()
}
