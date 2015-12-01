// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"v.io/jiri/jiritest"
	"v.io/jiri/profiles"
)

func addProfileAndTargets(t *testing.T, name string) {
	t1, _ := profiles.NewTargetWithEnv("cpu1-os1@1", "A=B,C=D")
	t2, _ := profiles.NewTargetWithEnv("cpu2-os2@bar", "A=B,C=D")
	if err := profiles.AddProfileTarget(name, t1); err != nil {
		t.Fatal(err)
	}
	t2.InstallationDir = "bar"
	if err := profiles.AddProfileTarget(name, t2); err != nil {
		t.Fatal(err)
	}
}

func tmpFile() string {
	dirname, err := ioutil.TempDir("", "pdb")
	if err != nil {
		panic(err)
	}
	return filepath.Join(dirname, "manifest")
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

func TestWrite(t *testing.T) {
	profiles.Clear()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	// test for no version being set.
	t1, _ := profiles.NewTargetWithEnv("cpu1-os1", "A=B,C=D")
	if err := profiles.AddProfileTarget("b", t1); err != nil {
		t.Fatal(err)
	}
	if err := profiles.Write(jirix, filename); err == nil || !strings.HasPrefix(err.Error(), "missing version for profile") {
		t.Fatalf("was expecing a missing version error, but got %v", err)
	}
	profiles.RemoveProfileTarget("b", t1)

	addProfileAndTargets(t, "b")
	addProfileAndTargets(t, "a")
	if err := profiles.Write(jirix, filename); err != nil {
		t.Fatal(err)
	}

	g, _ := ioutil.ReadFile(filename)
	w, _ := ioutil.ReadFile("./testdata/m1.xml")
	if got, want := removeDate(strings.TrimSpace(string(g))), strings.TrimSpace(string(w)); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRead(t *testing.T) {
	profiles.Clear()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	if err := profiles.Read(jirix, "./testdata/m1.xml"); err != nil {
		t.Fatal(err)
	}

	cmp := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i, s := range a {
			if s != b[i] {
				return false
			}
		}
		return true
	}
	names := profiles.Profiles()
	sort.Strings(names)
	if got, want := names, []string{"a", "b"}; !cmp(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	p := profiles.LookupProfile("a")
	if got, want := p.Targets()[0].OS(), "os1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := p.Targets()[1].Version(), "bar"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestInstallProfile(t *testing.T) {
	profiles.Clear()
	profiles.InstallProfile("a", "root1")
	profiles.InstallProfile("a", "root2")
	profile := profiles.LookupProfile("a")
	if got, want := profile.Root, "root1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadingV0(t *testing.T) {
	profiles.Clear()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	getProfiles := func() []*profiles.Profile {
		db := []*profiles.Profile{}
		names := profiles.Profiles()
		sort.Strings(names)
		for _, name := range names {
			db = append(db, profiles.LookupProfile(name))
		}
		return db
	}

	if err := profiles.Read(jirix, "./testdata/legacy.xml"); err != nil {
		t.Fatal(err)
	}

	if got, want := profiles.SchemaVersion(), profiles.Original; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	oprofiles := getProfiles()
	if got, want := len(oprofiles), 5; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	var t1 profiles.Target
	t1.Set("cpu-os@1")
	profiles.AddProfileTarget("__first", t1)

	if err := profiles.Write(jirix, filename); err != nil {
		t.Fatal(err)
	}

	if err := profiles.Read(jirix, filename); err != nil {
		t.Fatal(err)
	}

	if got, want := profiles.SchemaVersion(), profiles.V4; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	nprofiles := getProfiles()
	if got, want := len(nprofiles), 6; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	if got, want := nprofiles[0].Name, "__first"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	for i, v := range nprofiles[1:] {
		if got, want := v.Name, oprofiles[i].Name; got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func handleRelativePath(root profiles.RelativePath, s string) string {
	// Handle the transition from absolute to relative paths.
	if filepath.IsAbs(s) {
		return s
	}
	return root.RootJoin(s).Expand()
}

func TestReadingV3AndV4(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	for i, c := range []struct {
		filename, prefix, variable string
		version                    profiles.Version
	}{
		{"v3.xml", "", "", profiles.V3},
		{"v4.xml", fake.X.Root, "${JIRI_ROOT}", profiles.V4},
	} {
		ch, err := profiles.NewConfigHelper(fake.X, profiles.UseProfiles, filepath.Join("testdata", c.filename))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := profiles.SchemaVersion(), c.version; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		target, err := profiles.NewTarget("cpu1-os1@1")
		if err != nil {
			t.Fatal(err)
		}
		p := profiles.LookupProfile("a")
		// We need to expand the variable here for a V4 profile if we want
		// to get the full absolute path.
		if got, want := p.Root, c.variable+"/an/absolute/root"; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		lt := profiles.LookupProfileTarget("a", target)
		if got, want := lt.InstallationDir, c.variable+"/an/absolute/dir"; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		// The merged environment variables are expanded appropriately
		// internally by MergeEnvFromProfiles.
		ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), target, "a")
		if got, want := ch.Get("ABS"), "-I"+c.prefix+"/an/absolute/path"; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}
