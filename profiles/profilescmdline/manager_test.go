// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profilescmdline_test

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"v.io/jiri/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/profiles/profilesreader"
	"v.io/x/lib/envvar"
	"v.io/x/lib/gosh"
)

func TestManagerArgs(t *testing.T) {
	profilescmdline.Reset()
	p := parent
	profilescmdline.RegisterManagementCommands(&p, false, "", "", jiri.ProfilesRootDir)
	if got, want := len(p.Children), 5; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	type cl struct {
		args string
		n    int
	}
	cls := map[string]cl{
		"install":   cl{"--profiles-db=db --profiles-dir=root --target=arch-os --env=a=b,c=d --force=false", 5},
		"uninstall": cl{"--profiles-db=db --profiles-dir=root --target=arch-os --all-targets --v", 5},
		"cleanup":   cl{"--profiles-db=db --profiles-dir=root --gc --rm-all --v", 5},
		"update":    cl{"--profiles-db=db --profiles-dir=root -v", 3},
		"available": cl{"-v", 1},
	}
	for _, c := range p.Children {
		args := cls[c.Name].args
		if err := c.Flags.Parse(strings.Split(args, " ")); err != nil {
			t.Errorf("failed to parse for %s: %s: %v", c.Name, args, err)
			continue
		}
		if got, want := c.Flags.NFlag(), cls[c.Name].n; got != want {
			t.Errorf("%s: got %v, want %v", c.Name, got, want)
		}
	}
}

var (
	buildInstallersOnce, buildJiriOnce     sync.Once
	buildInstallersBinDir, buildJiriBinDir = "", ""
)

// TestMain must cleanup these directories created by this function.
func buildInstallers(t *testing.T) string {
	buildInstallersOnce.Do(func() {
		binDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatal(err)
		}
		sh := gosh.NewShell(t)
		defer sh.Cleanup()
		prefix := "v.io/jiri/profiles/profilescmdline/internal/"
		gosh.BuildGoPkg(sh, binDir, "v.io/jiri/cmd/jiri", "-o", "jiri")
		gosh.BuildGoPkg(sh, binDir, prefix+"i1", "-o", "jiri-profile-i1")
		gosh.BuildGoPkg(sh, binDir, prefix+"i2", "-o", "jiri-profile-i2")
		buildInstallersBinDir = binDir
	})
	return buildInstallersBinDir
}

func TestMain(m *testing.M) {
	flag.Parse()
	r := m.Run()
	if buildInstallersBinDir != "" {
		os.RemoveAll(buildInstallersBinDir)
	}
	os.Exit(r)
}

func createProfilesDB(t *testing.T, jirix *jiri.X) {
	if err := os.MkdirAll(jirix.ProfilesDBDir(), os.FileMode(0755)); err != nil {
		t.Fatalf("%s:%s", loc(), err)
	}
}

func buildJiri(t *testing.T) string {
	buildJiriOnce.Do(func() {
		binDir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatal(err)
		}
		sh := gosh.NewShell(t)
		defer sh.Cleanup()
		gosh.BuildGoPkg(sh, binDir, "v.io/jiri/cmd/jiri", "-o", "jiri")
		buildJiriBinDir = binDir
	})
	return buildJiriBinDir
}

func run(sh *gosh.Shell, dir, bin string, args ...string) string {
	cmd := sh.Cmd(filepath.Join(dir, bin), args...)
	if testing.Verbose() {
		cmd.PropagateOutput = true
	}
	return cmd.Stdout()
}

func TestManagerAvailable(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	dir, sh := buildInstallers(t), gosh.NewShell(t)
	createProfilesDB(t, fake.X)
	sh.Vars["JIRI_ROOT"] = fake.X.Root
	sh.Vars["PATH"] = envvar.PrependUsingSeparator(dir, os.Getenv("PATH"), ":")
	stdout := run(sh, dir, "jiri", "profile", "available", "-v")
	for _, installer := range []string{"i1", "i2"} {
		re := regexp.MustCompile("Available Subcommands:.*profile-" + installer + ".*\n")
		if got := stdout; !re.MatchString(got) {
			t.Errorf("%v does not match %v\n", got, re.String())
		}
		if got, want := stdout, installer+":eg"; !strings.Contains(got, want) {
			t.Errorf("%v does not contain %v\n", got, want)
		}
	}
	os.RemoveAll(filepath.Join(fake.X.Root, jiri.ProfilesDBDir))
	stdout = run(sh, dir, "jiri", "profile", "available", "-v")
	if got, want := strings.TrimSpace(stdout), "Available Subcommands:"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func loc() string {
	_, file, line, _ := runtime.Caller(2)
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func contains(t *testing.T, filename, want string) {
	o, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(o); got != want {
		t.Errorf("%s: %s: got %v, want %v", loc(), filename, got, want)
	}
}

func cat(msg, filename string) {
	o, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintln(os.Stderr, string(o))
	fmt.Fprintln(os.Stderr, msg)
}

func removeDate(s string) string {
	var result bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewBufferString(s))
	re := regexp.MustCompile("(.*) date=\".*\"")
	for scanner.Scan() {
		result.WriteString(re.ReplaceAllString(scanner.Text(), "$1"))
		result.WriteString("\n")
	}
	return strings.TrimSpace(result.String())
}

func cmpFiles(t *testing.T, gotFilename, wantFilename string) {
	g, err := ioutil.ReadFile(gotFilename)
	if err != nil {
		t.Fatalf("%s: %s", loc(), err)
	}
	w, err := ioutil.ReadFile(wantFilename)
	if err != nil {
		t.Fatalf("%s: %s", loc(), err)
	}
	if got, want := removeDate(strings.TrimSpace(string(g))), removeDate(strings.TrimSpace(string(w))); got != want {
		t.Errorf("%s: got %v, want %v from %q and %q", loc(), got, want, gotFilename, wantFilename)
	}
}

func TestManagerInstallUninstall(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	dir, sh := buildInstallers(t), gosh.NewShell(t)
	createProfilesDB(t, fake.X)
	sh.Vars["JIRI_ROOT"] = fake.X.Root
	sh.Vars["PATH"] = envvar.PrependUsingSeparator(dir, os.Getenv("PATH"), ":")

	run(sh, dir, "jiri", "profile", "list", "-v")

	i1 := filepath.Join(fake.X.Root, ".jiri_root/profile_db/i1")
	i2 := filepath.Join(fake.X.Root, ".jiri_root/profile_db/i2")

	run(sh, dir, "jiri", "profile", "install", "--target=arch-os", "i1:eg", "i2:eg")
	for _, installer := range []string{"i1", "i2"} {
		tdir := filepath.Join(fake.X.Root, jiri.ProfilesRootDir, installer, "eg", "arch_os")
		contains(t, filepath.Join(tdir, "version"), "3")
		contains(t, filepath.Join(tdir, "3"), "3")
	}
	cmpFiles(t, i1, filepath.Join("testdata", "i1a.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2a.xml"))

	run(sh, dir, "jiri", "profile", "install", "--target=arch-os@2", "i1:eg", "i2:eg")
	// Installs are idempotent.
	run(sh, dir, "jiri", "profile", "install", "--target=arch-os@2", "i1:eg", "i2:eg")
	for _, installer := range []string{"i1", "i2"} {
		tdir := filepath.Join(fake.X.Root, jiri.ProfilesRootDir, installer, "eg", "arch_os")
		contains(t, filepath.Join(tdir, "version"), "2")
		contains(t, filepath.Join(tdir, "3"), "3")
		contains(t, filepath.Join(tdir, "2"), "2")
	}

	cmpFiles(t, i1, filepath.Join("testdata", "i1b.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2b.xml"))

	run(sh, dir, "jiri", "profile", "uninstall", "--target=arch-os@2", "i1:eg", "i2:eg")
	for _, installer := range []string{"i1", "i2"} {
		tdir := filepath.Join(fake.X.Root, jiri.ProfilesRootDir, installer, "eg", "arch_os")
		contains(t, filepath.Join(tdir, "version"), "2")
		contains(t, filepath.Join(tdir, "3"), "3")
		if got, want := exists(filepath.Join(tdir, "2")), false; got != want {
			t.Errorf("%s: got %v, want %v", filepath.Join(tdir, "2"), got, want)
		}
	}
	cmpFiles(t, i1, filepath.Join("testdata", "i1c.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2c.xml"))

	// Put v2 back.
	run(sh, dir, "jiri", "profile", "list", "-v")
	run(sh, dir, "jiri", "profile", "install", "--target=arch-os@2", "i1:eg", "i2:eg")
	cmpFiles(t, i1, filepath.Join("testdata", "i1b.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2b.xml"))
}

func TestManagerUpdate(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	dir, sh := buildInstallers(t), gosh.NewShell(t)
	createProfilesDB(t, fake.X)
	sh.Vars["JIRI_ROOT"] = fake.X.Root
	sh.Vars["PATH"] = envvar.PrependUsingSeparator(dir, os.Getenv("PATH"), ":")

	i1 := filepath.Join(fake.X.ProfilesDBDir(), "i1")
	i2 := filepath.Join(fake.X.ProfilesDBDir(), "i2")

	run(sh, dir, "jiri", "profile", "install", "--target=arch-os@2", "i1:eg", "i2:eg")
	cmpFiles(t, i1, filepath.Join("testdata", "i1d.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2d.xml"))

	run(sh, dir, "jiri", "profile", "update")
	cmpFiles(t, i1, filepath.Join("testdata", "i1e.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2e.xml"))

	run(sh, dir, "jiri", "profile", "cleanup", "-gc")
	cmpFiles(t, i1, filepath.Join("testdata", "i1f.xml"))
	cmpFiles(t, i2, filepath.Join("testdata", "i2f.xml"))

	run(sh, dir, "jiri", "profile", "cleanup", "-rm-all")
	profiledir := filepath.Join(fake.X.Root, jiri.ProfilesRootDir)
	for _, dir := range []string{
		fake.X.ProfilesDBDir(),
		filepath.Join(profiledir, "i1"),
		filepath.Join(profiledir, "i2"),
	} {
		_, err := os.Stat(dir)
		if !os.IsNotExist(err) {
			t.Errorf("%v still exists: %v", dir, err)
		}
	}
	// Start over, make sure update is idempotent.
	createProfilesDB(t, fake.X)
	run(sh, dir, "jiri", "profile", "install", "--target=arch-os@2", "i1:eg")
	run(sh, dir, "jiri", "profile", "update")
	run(sh, dir, "jiri", "profile", "update")
	cmpFiles(t, i1, filepath.Join("testdata", "i1g.xml"))
	run(sh, dir, "jiri", "profile", "install", "--target=arch-os@4", "i1:eg")
	run(sh, dir, "jiri", "profile", "update")
	cmpFiles(t, i1, filepath.Join("testdata", "i1h.xml"))
}

// Test using a fake jiri root.
func TestJiriFakeRoot(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	profilesDBDir := filepath.Join(fake.X.Root, jiri.ProfilesDBDir)
	_ = cleanup
	pdb := profiles.NewDB()
	t1, err := profiles.NewTarget("cpu1-os1@1", "A=B,C=D")
	if err != nil {
		t.Fatal(err)
	}
	pdb.InstallProfile("test", "b", "")
	if err := pdb.AddProfileTarget("test", "b", t1); err != nil {
		t.Fatal(err)
	}
	if err := pdb.Write(fake.X, "test", profilesDBDir); err != nil {
		t.Fatal(err)
	}

	rd, err := profilesreader.NewReader(fake.X, profilesreader.UseProfiles, profilesDBDir)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := rd.ProfileNames(), []string{"test:b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	dir, sh := buildJiri(t), gosh.NewShell(t)
	sh.Vars["JIRI_ROOT"] = fake.X.Root
	sh.Vars["PATH"] = envvar.PrependUsingSeparator(dir, os.Getenv("PATH"), ":")
	run(sh, dir, "jiri", "profile", "list", "-v")
}
