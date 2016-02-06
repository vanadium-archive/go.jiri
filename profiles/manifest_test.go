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
	"reflect"
	"regexp"
	"strings"
	"testing"

	"v.io/jiri/jiritest"
	"v.io/jiri/profiles"
)

func addProfileAndTargets(t *testing.T, pdb *profiles.DB, name string) {
	t1, _ := profiles.NewTarget("cpu1-os1@1", "A=B,C=D")
	t2, _ := profiles.NewTarget("cpu2-os2@bar", "A=B,C=D")
	pdb.InstallProfile("test", name, "")
	if err := pdb.AddProfileTarget("test", name, t1); err != nil {
		t.Fatal(err)
	}
	t2.InstallationDir = "bar"
	if err := pdb.AddProfileTarget("test", name, t2); err != nil {
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
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	// test for no version being set.
	t1, _ := profiles.NewTarget("cpu1-os1", "A=B,C=D")
	pdb.InstallProfile("test", "b", "")
	if err := pdb.AddProfileTarget("test", "b", t1); err != nil {
		t.Fatal(err)
	}
	if err := pdb.Write(jirix, "test", filename); err == nil || !strings.HasPrefix(err.Error(), "missing version for profile") {
		t.Fatalf("was expecing a missing version error, but got %v", err)
	}
	pdb.RemoveProfileTarget("test", "b", t1)

	addProfileAndTargets(t, pdb, "b")
	addProfileAndTargets(t, pdb, "a")
	if err := pdb.Write(jirix, "test", filename); err != nil {
		t.Fatal(err)
	}

	g, _ := ioutil.ReadFile(filename)
	w, _ := ioutil.ReadFile("./testdata/m1.xml")
	if got, want := removeDate(strings.TrimSpace(string(g))), strings.TrimSpace(string(w)); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func exists(t *testing.T, filename string) bool {
	fi, err := os.Stat(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return false
	}
	return fi.Size() > 0
}

func TestWriteAndRename(t *testing.T) {
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	addProfileAndTargets(t, pdb, "b")

	if exists(t, filename) {
		t.Fatalf("%q  exists", filename)
	}
	if exists(t, filename+".prev") {
		t.Fatalf("%q  exists", filename+".prev")
	}
	if err := pdb.Write(jirix, "test", filename); err != nil {
		t.Fatal(err)
	}

	if !exists(t, filename) {
		t.Fatalf("%q  exists", filename)
	}
	if exists(t, filename+".prev") {
		t.Fatalf("%q  exists", filename+".prev")
	}

	if err := pdb.Write(jirix, "test", filename); err != nil {
		t.Fatal(err)
	}

	if !exists(t, filename) {
		t.Fatalf("%q does not exist", filename)
	}
	if !exists(t, filename+".prev") {
		t.Fatalf("%q does not exist", filename+".prev")
	}
}

func TestWriteDir(t *testing.T) {
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	dir, err := ioutil.TempDir("", "pdb-")
	if err != nil {
		t.Fatal(err)
	}

	t1, _ := profiles.NewTarget("cpu1-os1@1", "A=B,C=D")
	pdb.InstallProfile("i1", "b", "")
	if err := pdb.AddProfileTarget("i1", "b", t1); err != nil {
		t.Fatal(err)
	}

	t2, _ := profiles.NewTarget("cpu1-os1@2", "A=B,C=D")
	pdb.InstallProfile("i2", "b", "")
	if err := pdb.AddProfileTarget("i2", "b", t2); err != nil {
		t.Fatal(err)
	}

	// Write out i1 installer's data
	if err := pdb.Write(jirix, "i1", dir); err != nil {
		t.Fatal(err)
	}
	if f := filepath.Join(dir, "i1"); !exists(t, f) {
		t.Errorf("%s doesn't exist", f)
	}
	if f := filepath.Join(dir, "i2"); exists(t, f) {
		t.Errorf("%s exists", f)
	}

	// Write out i2 installer's data
	if err := pdb.Write(jirix, "i2", dir); err != nil {
		t.Fatal(err)
	}
	if f := filepath.Join(dir, "i1"); !exists(t, f) {
		t.Errorf("%s doesn't exist", f)
	}
	if f := filepath.Join(dir, "i2"); !exists(t, f) {
		t.Errorf("%s doesn't exist", f)
	}
}

func TestRead(t *testing.T) {
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	if err := pdb.Read(jirix, "./testdata/m1.xml"); err != nil {
		t.Fatal(err)
	}
	names := pdb.Names()
	if got, want := names, []string{"test:a", "test:b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	p := pdb.LookupProfile("test", "a")
	if got, want := p.Targets()[0].OS(), "os1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := p.Targets()[1].Version(), "bar"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadDir(t *testing.T) {
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	if err := pdb.Read(jirix, "./testdata/old_version"); err == nil || !strings.Contains(err.Error(), "files must be at version") {
		t.Fatalf("missing or wrong error: %v", err)
	}
	if err := pdb.Read(jirix, "./testdata/mismatched_versions"); err == nil || !strings.Contains(err.Error(), "files must have the same version") {
		t.Fatalf("missing or wrong error: %v", err)
	}
	if err := pdb.Read(jirix, "./testdata/db_dir"); err != nil {
		t.Fatal(err)
	}
	names := pdb.Names()
	if got, want := names, []string{"m1:a", "m1:b", "m2:a", "m2:b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	profile := pdb.LookupProfile("m2", "a")
	if got, want := profile.Root(), "root"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	profile = pdb.LookupProfile("m1", "b")
	if got, want := profile.Root(), "r1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadDirWithPrevFiles(t *testing.T) {
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	if err := pdb.Read(jirix, "./testdata/db_dir_with_prev"); err != nil {
		t.Fatal(err)
	}
	names := pdb.Names()
	if got, want := names, []string{"m1:a", "m2:b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	profile := pdb.LookupProfile("m1", "a")
	if got, want := profile.Root(), "root"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	profile = pdb.LookupProfile("m2", "b")
	if got, want := profile.Root(), "r2"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestInstallProfile(t *testing.T) {
	pdb := profiles.NewDB()
	pdb.InstallProfile("test", "a", "root1")
	pdb.InstallProfile("test", "a", "root2")
	profile := pdb.LookupProfile("test", "a")
	if got, want := profile.Root(), "root1"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestReadingV0(t *testing.T) {
	pdb := profiles.NewDB()
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	if err := pdb.Read(jirix, "./testdata/legacy.xml"); err != nil {
		t.Fatal(err)
	}

	if got, want := pdb.SchemaVersion(), profiles.Original; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	oprofiles := pdb.Profiles()
	if got, want := len(oprofiles), 5; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	var t1 profiles.Target
	t1.Set("cpu-os@1")
	pdb.InstallProfile("", "__first", "")
	if err := pdb.AddProfileTarget("", "__first", t1); err != nil {
		t.Fatal(err)
	}

	if err := pdb.Write(jirix, "", filename); err != nil {
		t.Fatal(err)
	}

	if err := pdb.Read(jirix, filename); err != nil {
		t.Fatal(err)
	}

	if got, want := pdb.SchemaVersion(), profiles.V5; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	nprofiles := pdb.Profiles()
	if got, want := len(nprofiles), 6; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got, want := nprofiles[0].Name(), "__first"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	for i, v := range nprofiles[1:] {
		if got, want := v.Name(), oprofiles[i].Name(); got != want {
			t.Errorf("got %v, want %v", got, want)
		}
	}
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
		pdb := profiles.NewDB()
		err := pdb.Read(fake.X, filepath.Join("testdata", c.filename))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := pdb.SchemaVersion(), c.version; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		target, err := profiles.NewTarget("cpu1-os1@1", "")
		if err != nil {
			t.Fatal(err)
		}
		p := pdb.LookupProfile("", "a")
		// We need to expand the variable here for a V4 profile if we want
		// to get the full absolute path.
		if got, want := p.Root(), c.variable+"/an/absolute/root"; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		lt := pdb.LookupProfileTarget("", "a", target)
		if got, want := lt.InstallationDir, c.variable+"/an/absolute/dir"; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}

func TestQualifiedNames(t *testing.T) {
	for i, c := range []struct{ i, p, e string }{
		{"a", "b", "a:b"},
		{"a", ":b", "a:b"},
		{"a", "", "a:"},
		{"", "b", "b"},
		{"", "", ""},
	} {
		if got, want := profiles.QualifiedProfileName(c.i, c.p), c.e; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
	for i, c := range []struct{ q, i, p string }{
		{"aa:bb", "aa", "bb"},
		{":bb", "", "bb"},
		{"bb", "", "bb"},
		{"", "", ""},
		{":bb", "", "bb"},
	} {
		gi, gp := profiles.SplitProfileName(c.q)
		if got, want := gi, c.i; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		if got, want := gp, c.p; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}

	if got, want := profiles.QualifiedProfileName("new", "old:bar"), "new:bar"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

}
