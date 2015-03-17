package envutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestToMap(t *testing.T) {
	tests := []struct {
		Slice []string
		Map   map[string]string
	}{
		{nil, nil},
		{[]string{``}, nil},
		{
			[]string{
				``,
				`A`,
				`B=`,
				`C=3`,
				`D==`,
				`E==5`,
				`F=6=`,
				`G=7=7`,
				`H="8"`,
			},
			map[string]string{
				`A`: ``,
				`B`: ``,
				`C`: `3`,
				`D`: `=`,
				`E`: `=5`,
				`F`: `6=`,
				`G`: `7=7`,
				`H`: `"8"`,
			},
		},
	}
	for _, test := range tests {
		if got, want := ToMap(test.Slice), test.Map; !reflect.DeepEqual(got, want) {
			t.Errorf("ToMap got %v, want %v", got, want)
		}
	}
}

func TestToSlice(t *testing.T) {
	tests := []struct {
		Map   map[string]string
		Slice []string
	}{
		{nil, nil},
		{map[string]string{``: ``}, nil},
		{map[string]string{``: `foo`}, nil},
		{map[string]string{``: `foo`}, nil},
		{
			map[string]string{
				``:  ``,
				`A`: ``,
				`B`: ``,
				`C`: `3`,
				`D`: `=`,
				`E`: `=5`,
				`F`: `6=`,
				`G`: `7=7`,
				`H`: `"8"`,
			},
			[]string{
				`A=`,
				`B=`,
				`C=3`,
				`D==`,
				`E==5`,
				`F=6=`,
				`G=7=7`,
				`H="8"`,
			},
		},
	}
	for _, test := range tests {
		if got, want := ToSlice(test.Map), test.Slice; !reflect.DeepEqual(got, want) {
			t.Errorf("ToSlice got %v, want %v", got, want)
		}
	}
}

func TestToQuotedSlice(t *testing.T) {
	tests := []struct {
		Map   map[string]string
		Slice []string
	}{
		{nil, nil},
		{map[string]string{``: ``}, nil},
		{map[string]string{``: `foo`}, nil},
		{map[string]string{``: `foo`}, nil},
		{
			map[string]string{
				``:  ``,
				`A`: ``,
				`B`: ``,
				`C`: `3`,
				`D`: `=`,
				`E`: `=5`,
				`F`: `6=`,
				`G`: `7=7`,
				`H`: `"8"`,
			},
			[]string{
				`A=""`,
				`B=""`,
				`C="3"`,
				`D="="`,
				`E="=5"`,
				`F="6="`,
				`G="7=7"`,
				`H="\"8\""`,
			},
		},
	}
	for _, test := range tests {
		if got, want := ToQuotedSlice(test.Map), test.Slice; !reflect.DeepEqual(got, want) {
			t.Errorf("ToQuotedSlice got %v, want %v", got, want)
		}
	}
}

func TestCopyEmpty(t *testing.T) {
	tests := []map[string]string{
		nil,
		{},
	}
	for _, test := range tests {
		got := Copy(test)
		if got == nil {
			t.Errorf("Copy(%#v) got nil, which should never happen", test)
		}
		if got, want := len(got), 0; got != want {
			t.Errorf("Copy(%#v) got len %d, want %d", test, got, want)
		}
	}
}

func TestCopy(t *testing.T) {
	tests := []map[string]string{
		{},
		{"A": "1", "B": "2"},
		{"A": "1", "B": "2", "C": "3"},
	}
	for _, want := range tests {
		got := Copy(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Copy got %v, want %v", got, want)
		}
		// Make sure we haven't just returned the original input map.
		got["test"] = "foo"
		if reflect.DeepEqual(got, want) {
			t.Errorf("Copy got %v, which is the input map", got)
		}
	}
}

type keyVal struct {
	Key, Val string
}

func testSnapshotGet(t *testing.T, s *Snapshot, tests []keyVal) {
	for _, kv := range tests {
		if got, want := s.Get(kv.Key), kv.Val; got != want {
			t.Errorf(`Get(%q) got %v, want %v`, kv.Key, got, want)
		}
	}
}

type keyTok struct {
	Key, Sep string
	Tok      []string
}

func testSnapshotGetTokens(t *testing.T, s *Snapshot, tests []keyTok) {
	for _, kt := range tests {
		if got, want := s.GetTokens(kt.Key, kt.Sep), kt.Tok; !reflect.DeepEqual(got, want) {
			t.Errorf(`GetTokens(%q, %q) got %v, want %v`, kt.Key, kt.Sep, got, want)
		}
	}
}

func TestSnapshotEmpty(t *testing.T) {
	s := NewSnapshot(nil)
	if got, want := s.Map(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("Map got %v, want %v", got, want)
	}
	if got, want := s.Slice(), []string(nil); !reflect.DeepEqual(got, want) {
		t.Errorf("Slice got %v, want %v", got, want)
	}
	if got, want := s.BaseMap(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseMap got %v, want %v", got, want)
	}
	if got, want := s.DeltaMap(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("DeltaMap got %v, want %v", got, want)
	}
	testSnapshotGet(t, s, []keyVal{{"noexist", ""}})
	testSnapshotGetTokens(t, s, []keyTok{{"noexist", ":", nil}})
}

func TestSnapshotNoSet(t *testing.T) {
	base := map[string]string{"A": "", "B": "foo", "C": "1:2:3"}
	s := NewSnapshot(base)
	if got, want := s.Map(), base; !reflect.DeepEqual(got, want) {
		t.Errorf("Map got %v, want %v", got, want)
	}
	if got, want := s.Slice(), []string{"A=", "B=foo", "C=1:2:3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Slice got %v, want %v", got, want)
	}
	if got, want := s.BaseMap(), base; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseMap got %v, want %v", got, want)
	}
	if got, want := s.DeltaMap(), map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("DeltaMap got %v, want %v", got, want)
	}
	testSnapshotGet(t, s, []keyVal{
		{"A", ""},
		{"B", "foo"},
		{"C", "1:2:3"},
		{"noexist", ""},
	})
	testSnapshotGetTokens(t, s, []keyTok{
		{"A", ":", nil},
		{"A", " ", nil},
		{"B", ":", []string{"foo"}},
		{"B", " ", []string{"foo"}},
		{"C", ":", []string{"1", "2", "3"}},
		{"C", " ", []string{"1:2:3"}},
		{"noexist", ":", nil},
		{"noexist", " ", nil},
	})
}

func TestSnapshotWithSet(t *testing.T) {
	base := map[string]string{"A": "", "B": "foo", "C": "1:2:3"}
	s := NewSnapshot(base)
	s.SetTokens("B", []string{"a", "b", "c"}, ":")
	s.Set("C", "bar")
	s.Set("D", "baz")
	final := map[string]string{"A": "", "B": "a:b:c", "C": "bar", "D": "baz"}
	if got, want := s.Map(), final; !reflect.DeepEqual(got, want) {
		t.Errorf("Map got %v, want %v", got, want)
	}
	if got, want := s.Slice(), []string{"A=", "B=a:b:c", "C=bar", "D=baz"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Slice got %v, want %v", got, want)
	}
	if got, want := s.BaseMap(), base; !reflect.DeepEqual(got, want) {
		t.Errorf("BaseMap got %v, want %v", got, want)
	}
	delta := Copy(final)
	delete(delta, "A")
	if got, want := s.DeltaMap(), delta; !reflect.DeepEqual(got, want) {
		t.Errorf("DeltaMap got %v, want %v", got, want)
	}
	testSnapshotGet(t, s, []keyVal{
		{"A", ""},
		{"B", "a:b:c"},
		{"C", "bar"},
		{"D", "baz"},
		{"noexist", ""},
	})
	testSnapshotGetTokens(t, s, []keyTok{
		{"A", ":", nil},
		{"A", " ", nil},
		{"B", ":", []string{"a", "b", "c"}},
		{"B", " ", []string{"a:b:c"}},
		{"C", ":", []string{"bar"}},
		{"C", " ", []string{"bar"}},
		{"D", ":", []string{"baz"}},
		{"D", " ", []string{"baz"}},
		{"noexist", ":", nil},
		{"noexist", " ", nil},
	})
}

func TestNewSnapshotFromOS(t *testing.T) {
	// Just set an environment variable and make sure it shows up.
	const testKey, testVal = "OS_ENV_TEST_KEY", "OS_ENV_TEST_VAL"
	if err := os.Setenv(testKey, testVal); err != nil {
		t.Fatalf("Setenv(%q, %q) failed: %v", testKey, testVal, err)
	}
	s := NewSnapshotFromOS()
	if got, want := s.Get(testKey), testVal; got != want {
		t.Errorf("Get(%q) got %q, want %q", testKey, got, want)
	}
}

func pathsMatch(t *testing.T, path1, path2 string) bool {
	eval1, err := filepath.EvalSymlinks(path1)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", path1, err)
	}
	eval2, err := filepath.EvalSymlinks(path2)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", path2, err)
	}
	return eval1 == eval2
}

// TestLookPathCommandOK checks that LookPath() succeeds when given an
// existing command.
func TestLookPathCommandOK(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	cmd := "vanadium-unlikely-binary-name"
	absPath := filepath.Join(tmpDir, cmd)

	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0755))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	command := filepath.Base(absPath)
	got, err := s.LookPath(command)
	if err != nil {
		t.Fatalf("LookPath(%v) failed: %v", command, err)
	}
	if want := absPath; !pathsMatch(t, got, want) {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

// TestLookPathCommandFail checks that LookPath() fails when given a
// non-existing command.
func TestLookPathCommandFail(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "vanadium-unlikely-binary-name")
	if _, err := s.LookPath(filepath.Base(absPath)); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", filepath.Base(absPath))
	}
}

// TestLookPathAbsoluteOk checks that LookPath() succeeds when given
// an existing absolute path.
func TestLookPathAbsoluteOK(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "vanadium-unlikely-binary-name")
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0755))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	got, err := s.LookPath(absPath)
	if err != nil {
		t.Fatalf("LookPath(%v) failed: %v", absPath, err)
	}
	if want := absPath; !pathsMatch(t, got, want) {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

// TestLookPathAbsoluteFail checks that LookPath() fails when given a
// non-existing absolute path.
func TestLookPathAbsoluteFail(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "vanadium-unlikely-binary-name")
	if _, err := s.LookPath(absPath); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", absPath)
	}
}

// TestLookPathAbsoluteExecFail checks that LookPath() fails when
// given an existing absolute path to a non-executable file.
func TestLookPathAbsoluteExecFail(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "vanadium-unlikely-binary-name")
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	if _, err := s.LookPath(absPath); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", absPath)
	}
}

// TestLookPathRelativeOK checks that LookPath() succeeds when given
// an existing relative path.
func TestLookPathRelativeOK(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	cmd := "vanadium-unlikely-binary-name"
	absPath := filepath.Join(tmpDir, cmd)
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0755))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	relPath := "." + string(os.PathSeparator) + filepath.Base(absPath)
	got, err := s.LookPath(relPath)
	if err != nil {
		t.Fatalf("LookPath(%v) failed: %v", relPath, err)
	}
	if want := absPath; !pathsMatch(t, got, want) {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

// TestLookPathRelativeFail checks that LookPath() fails when given a
// non-existing relative path.
func TestLookPathRelativeFail(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "vanadium-unlikely-binary-name")
	relPath := "." + string(os.PathSeparator) + filepath.Base(absPath)
	if _, err := s.LookPath(relPath); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", relPath)
	}
}

// TestLookPathRelativeExecFail checks that LookPath() fails when
// given an existing relative path to a non-executable file.
func TestLookPathRelativeExecFail(t *testing.T) {
	s := NewSnapshotFromOS()
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	s.Set("PATH", s.Get("PATH")+string(os.PathListSeparator)+tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "vanadium-unlikely-binary-name")
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	relPath := "." + string(os.PathSeparator) + filepath.Base(absPath)
	if _, err := s.LookPath(relPath); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", relPath)
	}
}
