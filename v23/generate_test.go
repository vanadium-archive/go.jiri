package main

import (
	"bufio"
	"bytes"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go/ast"
	"go/parser"
	"go/token"
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

func TestV23Generate(t *testing.T) {
	cases := []struct {
		dir, output        string
		internal, external []string
		testOutput         []string
	}{
		// an empty package.
		{"testdata/empty", "",
			[]string{"TestMain"},
			nil,
			nil,
		},
		// has a TestMain and a single module, hence the init function.
		{"testdata/has_main", "",
			nil,
			[]string{"init"},
			[]string{"TestHasMain"},
		},
		// has internal tests only
		{"testdata/internal_only", "",
			[]string{"TestMain", "init"},
			nil,
			[]string{"TestModulesInternalOnly"},
		},
		// has external modules only
		{"testdata/external_only", "",
			nil,
			[]string{"TestMain", "init"},
			[]string{"TestExternalOnly"},
		},
		// has V23 tests and internal+external modules
		{"testdata/one", "",
			[]string{"TestMain", "init"},
			[]string{"TestV23OneA", "TestV23OneB", "init"},
			[]string{
				"TestModulesOneAndTwo",
				"TestModulesOneExt",
				"TestV23OneA",
				"TestV23OneB",
				"TestV23TestMain"},
		},
		// test the -output flag.
		{"testdata/filename", "other",
			[]string{"TestMain", "init"},
			[]string{"TestV23Filename"},
			[]string{"TestInternalFilename", "TestV23Filename"},
		},
		{"testdata/modules_and_v23", "",
			[]string{"TestMain", "init"},
			[]string{"TestV23ModulesAndV23A", "TestV23ModulesAndV23B", "init"},
			[]string{"TestModulesAndV23Ext",
				"TestModulesAndV23Int",
				"TestV23ModulesAndV23A",
				"TestV23ModulesAndV23B"},
		},
		{"testdata/modules_only", "",
			[]string{"TestMain", "init"},
			[]string{"init"},
			[]string{"TestModulesOnlyExt", "TestModulesOnlyInt"},
		},
		{"testdata/v23_only", "",
			nil,
			[]string{"TestMain", "TestV23V23OnlyA", "TestV23V23OnlyB"},
			[]string{"TestV23V23OnlyA", "TestV23V23OnlyB"},
		},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmdV23Generate.Init(nil, os.Stdout, os.Stderr)
	for _, c := range cases {
		if err := os.Chdir(c.dir); err != nil {
			t.Fatal(err)
		}
		t.Logf("test: %v", c.dir)
		output := c.output
		if len(output) == 0 {
			output = "v23"
		}
		if err := cmdV23Generate.Execute([]string{"--prefix=" + output}); err != nil {
			t.Fatal(err)
		}
		// parseFile returns nil if the file doesn't exist, which must
		// be matched by nil in the internal/external fields in the cases.
		if got, want := parseFile(t, output+"_internal_test.go"), c.internal; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got: %v, want: %#v", c.dir, got, want)
		}
		if got, want := parseFile(t, output+"_test.go"), c.external; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got: %v, want: %#v", c.dir, got, want)
		}

		testCmd := *cmdGo
		var stdout, stderr bytes.Buffer
		testCmd.Init(nil, &stdout, &stderr)
		if err := runGo(&testCmd, []string{"test", "-v", "--v23.tests"}); err != nil {
			t.Log(stderr.String())
			t.Fatalf("%s: %v", c.dir, err)
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
			t.Fatalf("%s: got %v, want %v", c.dir, got, want)
		}

		l := 0
		for _, prefix := range []string{"--- PASS: ", "=== RUN "} {
			for _, fn := range c.testOutput {
				got, want := lines[l], prefix+fn
				if !strings.HasPrefix(got, want) {
					t.Fatalf("%s: expected %q to start with %q", c.dir, got, want)
				}
				l++
			}
		}
		if got, want := lines[l], "PASS"; got != want {
			t.Fatalf("%s: got %v, want %v", c.dir, got, want)
		}
		l++
		got := lines[l]
		if !strings.HasPrefix(got, "ok") {
			t.Fatalf("%s: line %q doesn't start with \"ok\"", c.dir, got)
		}

		if !strings.Contains(got, c.dir) {
			t.Fatalf("%s: line %q doesn't contain %q", c.dir, c.dir)
		}

		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	}

}
