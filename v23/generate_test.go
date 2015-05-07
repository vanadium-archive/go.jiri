// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go/ast"
	"go/build"
	"go/parser"
	"go/token"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

func fnNames(decls []ast.Decl) []string {
	names := []string{}
	for _, d := range decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			names = append(names, fn.Name.Name)
		}
	}
	sort.Strings(names)
	return names
}

func parseFile(t *testing.T, file string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	return fnNames(f.Decls)
}

func TestMain(m *testing.M) {
	ctx := tool.NewDefaultContext()
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		panic(err)
	}
	build.Default.GOPATH = env.Get("GOPATH")
	os.Exit(m.Run())
}

func importPackage(t *testing.T, path string) *build.Package {
	bpkg, err := build.Import(path, ".", build.ImportMode(build.ImportComment))
	if err != nil {
		t.Fatal(err)
	}
	return bpkg
}

func cmpImports(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	sort.Strings(got)
	sort.Strings(want)
	for i, g := range got {
		if g != want[i] {
			return false
		}
	}
	return true
}

func TestV23Generate(t *testing.T) {
	sysImports := []string{"fmt", "io", "os", "testing"}
	modulesImports := []string{"v.io/x/ref/test/modules"}
	vioImports := []string{"v.io/x/ref/profiles", "v.io/x/ref/test"}
	v23Imports := []string{"v.io/x/ref/test", "v.io/x/ref/test/v23tests", "v.io/x/ref/profiles"}

	common := append(sysImports, modulesImports...)
	usesModules := append([]string{}, common...)
	usesModules = append(usesModules, vioImports...)
	usesModulesAndV23Tests := append([]string{}, common...)
	usesModulesAndV23Tests = append(usesModulesAndV23Tests, v23Imports...)
	usesModulesAndV23TestsNoProfile := append([]string{}, common...)
	usesModulesAndV23TestsNoProfile = append(usesModulesAndV23TestsNoProfile, "v.io/x/ref/test/v23tests")
	testdata := "v.io/x/devtools/v23/testdata/"
	middle := testdata + "transitive/middle"

	cases := []struct {
		dir, output                      string
		internal, external               []string
		testOutput                       []string
		internalImports, externalImports []string
	}{
		// an empty package.
		{"empty", "",
			[]string{"TestMain"},
			nil,
			nil,
			[]string{"os", "testing", "v.io/x/ref/test"},
			nil,
		},
		// has a TestMain and a single module, hence the init function.
		{"has_main", "",
			nil,
			[]string{"init"},
			[]string{"TestHasMain"},
			nil,
			usesModules,
		},
		// has internal tests only
		{"internal_only", "",
			[]string{"TestMain", "init"},
			nil,
			[]string{"TestModulesInternalOnly"},
			usesModules,
			nil,
		},
		// has external modules only
		{"external_only", "",
			nil,
			[]string{"TestMain", "init"},
			[]string{"TestExternalOnly"},
			nil,
			usesModules,
		},
		// has V23 tests and internal+external modules
		{"one", "",
			[]string{"TestMain", "init"},
			[]string{"TestV23OneA", "TestV23OneB", "init"},
			[]string{
				"TestModulesOneAndTwo",
				"TestModulesOneExt",
				"TestV23OneA",
				"TestV23OneB",
				"TestV23TestMain"},
			usesModulesAndV23Tests,
			usesModulesAndV23TestsNoProfile,
		},
		// test the -output flag.
		{"filename", "other",
			[]string{"TestMain", "init"},
			[]string{"TestV23Filename"},
			[]string{"TestInternalFilename", "TestV23Filename"},
			append(append([]string{}, common...), "v.io/x/ref/test", "v.io/x/ref/test/v23tests"),
			[]string{"testing", "v.io/x/ref/test/v23tests", "v.io/x/ref/profiles"},
		},
		{"modules_and_v23", "",
			[]string{"TestMain", "init"},
			[]string{"TestV23ModulesAndV23A", "TestV23ModulesAndV23B", "init"},
			[]string{"TestModulesAndV23Ext",
				"TestModulesAndV23Int",
				"TestV23ModulesAndV23A",
				"TestV23ModulesAndV23B"},
			usesModulesAndV23Tests,
			usesModulesAndV23TestsNoProfile,
		},
		{"modules_only", "",
			[]string{"TestMain", "init"},
			[]string{"init"},
			[]string{"TestModulesOnlyExt", "TestModulesOnlyInt"},
			usesModules,
			append(append([]string{}, common...), "v.io/x/ref/profiles"),
		},
		{"v23_only", "",
			nil,
			[]string{"TestMain", "TestV23V23OnlyA", "TestV23V23OnlyB"},
			[]string{"TestV23V23OnlyA", "TestV23V23OnlyB"},
			nil,
			append(append([]string{}, "os", "testing"), v23Imports...),
		},
		{"transitive", "",
			[]string{"TestMain"},
			nil,
			[]string{"TestModulesInternalOnly"},
			append(append(append([]string{}, "fmt", "os", "testing", middle), modulesImports...), vioImports...),
			nil,
		},
		{"transitive_no_use", "",
			[]string{"TestMain"},
			nil,
			[]string{"TestWithoutModules"},
			append(append([]string{}, "os", "testing", middle), vioImports...),
			nil,
		},
		{"transitive_external", "",
			nil,
			[]string{"TestMain", "TestV23OneA"},
			[]string{"TestModulesExternal", "TestV23OneA"},
			[]string{
				"os", "testing", middle, "v.io/x/ref/test/modules", "v.io/x/ref/profiles"},
			append([]string{"fmt", "os", "testing", testdata + "transitive_external", "v.io/x/ref/test/modules"}, v23Imports...),
		},
		{"internal_transitive_external", "",
			[]string{"TestMain"},
			nil,
			[]string{"TestInternal", "TestModulesExternal"},
			[]string{
				"fmt", "os", "testing", middle, "v.io/x/ref/test/modules", "v.io/x/ref/test", "v.io/x/ref/profiles"},
			[]string{"testing", testdata + "internal_transitive_external", "v.io/x/ref/profiles"},
		},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmdTestGenerate.Init(nil, os.Stdout, os.Stderr)
	for _, c := range cases {
		dir := filepath.Join("testdata", c.dir)
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		t.Logf("test: %v", dir)
		output := c.output
		if len(output) == 0 {
			output = "v23"
		}
		if err := cmdTestGenerate.Execute([]string{"--prefix=" + output}); err != nil {
			t.Fatal(err)
		}
		// parseFile returns nil if the file doesn't exist, which must
		// be matched by nil in the internal/external fields in the cases.
		if got, want := parseFile(t, output+"_internal_test.go"), c.internal; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got: %v, want: %#v", dir, got, want)
		}
		if got, want := parseFile(t, output+"_test.go"), c.external; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got: %v, want: %#v", dir, got, want)
		}

		testCmd := *cmdGo
		var stdout, stderr bytes.Buffer
		testCmd.Init(nil, &stdout, &stderr)
		if err := runGo(&testCmd, []string{"test", "-v", "--v23.tests"}); err != nil {
			t.Log(stderr.String())
			t.Fatalf("%s: %v", dir, err)
		}

		scanner := bufio.NewScanner(&stdout)
		lines := []string{}
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		sort.Strings(lines)
		// Lines looks like:
		// --- PASS: Test1
		// --- PASS: Test2
		// === RUN Test1
		// === RUN Test2
		// PASS
		// ok v.io.... <time>

		if got, want := len(lines), (len(c.testOutput)*2)+2; got != want {
			t.Fatalf("%s: got %v, want %v", dir, got, want)
		}

		l := 0
		for _, prefix := range []string{"--- PASS: ", "=== RUN "} {
			for _, fn := range c.testOutput {
				got, want := lines[l], prefix+fn
				if !strings.HasPrefix(got, want) {
					t.Fatalf("%s: expected %q to start with %q", dir, got, want)
				}
				l++
			}
		}
		if got, want := lines[l], "PASS"; got != want {
			t.Fatalf("%s: got %v, want %v", dir, got, want)
		}
		l++
		got := lines[l]
		if !strings.HasPrefix(got, "ok") {
			t.Fatalf("%s: line %q doesn't start with \"ok\"", dir, got)
		}

		if !strings.Contains(got, dir) {
			t.Fatalf("%s: line %q doesn't contain %q", dir, dir)
		}

		bpkg := importPackage(t, "v.io/x/devtools/v23/testdata/"+c.dir)
		if got, want := bpkg.TestImports, c.internalImports; !cmpImports(got, want) {
			t.Fatalf("got %v, want %v,", got, want)
		}
		if got, want := bpkg.XTestImports, c.externalImports; !cmpImports(got, want) {
			t.Fatalf("got %v, want %v,", got, want)
		}
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	}
}
