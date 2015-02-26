package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"v.io/lib/cmdline"
	"v.io/x/devtools/lib/collect"
)

var cmdV23Generate = &cmdline.Command{
	Run:   runV23Generate,
	Name:  "generate",
	Short: "Generates supporting code for v23 integration tests.",
	Long: `
The generate subcommand supports the vanadium integration test
framework and unit tests by generating go files that contain supporting
code. v23 test generate is intended to be invoked via the
'go generate' mechanism and the resulting files are to be checked in.

Integration tests are functions of the form shown below that are defined
in 'external' tests (i.e. those occurring in _test packages, rather than
being part of the package being tested). This ensures that integration
tests are isolated from the packages being tested and can be moved to their
own package if need be. Integration tests have the following form:

    func V23Test<x> (i *v23tests.T)

    'v23 test generate' operates as follows:

In addition, some commonly used functionality in vanadium unit tests
is streamlined. Arguably this should be in a separate command/file but
for now they are lumped together. The additional functionality is as
follows:

1. v.io/veyron/lib/modules requires the use of an explicit
   registration mechanism. 'v23 test generate' automatically
   generates these registration functions for any test function matches
   the modules.Main signature.

   For:
   // SubProc does the following...
   // Usage: <a> <b>...
   func SubProc(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error

   It will generate:

   modules.RegisterChild("SubProc",` + "`" + `SubProc does the following...
Usage: <a> <b>...` + "`" + `, SubProc)

2. 'TestMain' is used as the entry point for all vanadium tests, integration
   and otherwise. v23 will generate an appropriate version of this if one is
   not already defined. TestMain is 'special' in that only one definiton can
   occur across both the internal and external test packages. This is a
   consequence of how the go testing system is implemented.
`,

	// TODO(cnicolaou): once the initial deployment is done, revisit the
	// this functionality and possibly dissallow the 'if this doesn't exist
	// generate it' behaviour and instead always generate the required helper
	// functions.

	ArgsName: "[packages]",
	ArgsLong: "list of go packages"}

const defaultV23TestPrefix = "v23"

func runV23Generate(command *cmdline.Command, args []string) error {
	// TODO(cnicolaou): use http://godoc.org/golang.org/x/tools/go/loader
	// to replace accessing the AST directly. In the meantime make sure
	// the command line API is consistent with that change.

	if len(args) > 1 || (len(args) == 1 && args[0] != ".") {
		return command.UsageErrorf("unexpected or wrong arguments, currently only . is supported as a package name.")
	}
	fi, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
	candidates := []string{}
	re := regexp.MustCompile(".*_test.go")
	for _, f := range fi {
		if f.IsDir() {
			continue
		}
		if re.MatchString(f.Name()) {
			candidates = append(candidates, f.Name())
		}
	}

	v23Tests := []string{}

	internalModules := []moduleCommand{}
	externalModules := []moduleCommand{}

	hasTestMain := false
	packageName := ""

	externalFile := prefixFlag + "_test.go"
	internalFile := prefixFlag + "_internal_test.go"

	re = regexp.MustCompile(`V23Test(.*)`)
	fset := token.NewFileSet() // positions are relative to fset
	for _, file := range candidates {
		// Ignore the files we are generating.
		if file == externalFile || file == internalFile {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		// An external test package is one named <pkg>_test.
		isExternal := strings.HasSuffix(f.Name.Name, "_test")
		if len(packageName) == 0 {
			packageName = strings.TrimSuffix(f.Name.Name, "_test")
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}

			// If this function matches the declaration for modules.Main,
			// keep track of the names and comments associated with
			// such functions so that we can generate calls to
			// modules.RegisterChild for them.
			if n, c := isModulesMain(fn); len(n) > 0 {

				if isExternal {
					externalModules = append(externalModules, moduleCommand{n, c})
				} else {
					internalModules = append(internalModules, moduleCommand{n, c})

				}
			}

			// If this function is the testing TestMain then
			// keep track of the fact that we've seen it.
			if isTestMain(fn) {
				hasTestMain = true
			}
			name := fn.Name.String()
			if parts := re.FindStringSubmatch(name); isExternal && len(parts) == 2 {
				v23Tests = append(v23Tests, parts[1])
			}
		}
	}

	hasV23Tests := len(v23Tests) > 0
	needInternalFile := len(internalModules) > 0
	needExternalFile := len(externalModules) > 0 || hasV23Tests

	// TestMain is special in that it can only occur once even across
	// internal and external test packages. If if it doesn't occur
	// in either, we want to make sure we write it out in the internal
	// package.
	if !hasTestMain && !needInternalFile && !needExternalFile {
		needInternalFile = true
	}

	if needInternalFile {
		if err := writeInternalFile(internalFile, packageName, !hasTestMain, hasV23Tests, internalModules); err != nil {
			return err
		}
		hasTestMain = true
	}

	if needExternalFile {
		return writeExternalFile(externalFile, packageName, !hasTestMain, externalModules, v23Tests)
	}
	return nil
}

func isModulesMain(d ast.Decl) (string, string) {
	fn, ok := d.(*ast.FuncDecl)
	if !ok {
		return "", ""
	}

	if fn.Type == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return "", ""
	}
	name := fn.Name.Name

	typeNames := func(fl *ast.FieldList) []string {
		names := []string{}
		for _, f := range fl.List {
			switch v := f.Type.(type) {
			case *ast.Ident:
				names = append(names, v.Name)
			case *ast.SelectorExpr:
				// Deal with 'a, b type' parameters.
				for _, _ = range f.Names {
					if pkg, ok := v.X.(*ast.Ident); ok {
						names = append(names, pkg.Name+"."+v.Sel.Name)
					}
				}
			case *ast.MapType:
				if t, ok := v.Key.(*ast.Ident); !ok || t.Name != "string" {
					break
				}
				if t, ok := v.Value.(*ast.Ident); !ok || t.Name != "string" {
					break
				}
				names = append(names, "map[string]string")
			case *ast.Ellipsis:
				if t, ok := v.Elt.(*ast.Ident); !ok || t.Name != "string" {
					break
				}
				names = append(names, "...string")
			}
		}
		return names
	}

	cmp := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i, av := range a {
			if av != b[i] {
				return false
			}
		}
		return true
	}

	comments := func(cg *ast.CommentGroup) string {
		if cg == nil {
			return ""
		}
		c := ""
		for _, l := range cg.List {
			t := strings.TrimPrefix(l.Text, "//")
			c += strings.TrimSpace(t) + "\n"
		}
		return strings.TrimSuffix(c, "\n")
	}

	// the Modules.Main signature is as follows:
	// type Main func(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error
	results := []string{"error"}
	parameters := []string{"io.Reader", "io.Writer", "io.Writer", "map[string]string", "...string"}
	_, _ = results, parameters

	p := typeNames(fn.Type.Params)
	r := typeNames(fn.Type.Results)

	if !cmp(results, r) || !cmp(parameters, p) {
		return "", ""
	}
	return name, comments(fn.Doc)
}

func isTestMain(fn *ast.FuncDecl) bool {
	// TODO(cnicolaou): check the signature as well as the name
	if fn.Name.Name != "TestMain" {
		return false
	}
	return true
}

type moduleCommand struct {
	name, comment string
}

// writeInternalFile writes a generated test file that is inside the package.
// It cannot contain integration tests.
func writeInternalFile(fileName string, packageName string, needsTestMain, hasV23Tests bool, modules []moduleCommand) (e error) {

	hasModules := len(modules) > 0

	if !needsTestMain && !hasModules {
		return nil
	}

	out, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return out.Close() }, &e)

	writeHeader(out)
	fmt.Fprintf(out, "package %s\n\n", packageName)

	if needsTestMain {
		if hasModules {
			fmt.Fprintln(out, `import "fmt"`)
		}
		fmt.Fprintln(out, `import "testing"`)
		fmt.Fprintln(out, `import "os"`)
		fmt.Fprintln(out)
	}

	if hasModules {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/modules"`)
	}

	if needsTestMain {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil"`)
		if hasV23Tests {
			fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil/v23tests"`)
		}
	}

	if hasModules {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "func init() {")
		writeModuleRegistration(out, modules)
		fmt.Fprintln(out, "}")
	}

	if needsTestMain {
		writeTestMain(out, hasModules, hasV23Tests)
	}

	return nil
}

// writeExternalFile write a generated test file that is outside the package.
// It can contain intgreation tests.
func writeExternalFile(fileName string, packageName string, needsTestMain bool, modules []moduleCommand, v23Tests []string) (e error) {

	hasV23Tests := len(v23Tests) > 0
	hasModules := len(modules) > 0
	if !needsTestMain && !hasModules && !hasV23Tests {
		return nil
	}

	out, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return out.Close() }, &e)

	writeHeader(out)
	fmt.Fprintf(out, "package %s_test\n\n", packageName)

	trailingLine := false
	if needsTestMain && hasModules {
		fmt.Fprintln(out, `import "fmt"`)
		trailingLine = true
	}
	if needsTestMain || hasV23Tests {
		fmt.Fprintln(out, `import "testing"`)
		trailingLine = true
	}
	if needsTestMain {
		fmt.Fprintln(out, `import "os"`)
		trailingLine = true
	}
	if trailingLine {
		fmt.Fprintln(out)
	}

	if hasModules {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/modules"`)
	}

	if needsTestMain {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil"`)
	}

	if hasV23Tests {
		fmt.Fprintln(out, `import "v.io/core/veyron/lib/testutil/v23tests"`)
	}

	if hasModules {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "func init() {")
		writeModuleRegistration(out, modules)
		fmt.Fprintln(out, "}")
	}

	if needsTestMain {
		writeTestMain(out, hasModules, hasV23Tests)
	}

	// integration test wrappers.
	for _, t := range v23Tests {
		fmt.Fprintf(out, "\nfunc TestV23%s(t *testing.T) {\n", t)
		fmt.Fprintf(out, "\tv23tests.RunTest(t, V23Test%s)\n}\n", t)
	}
	return nil
}

func writeHeader(out io.Writer) {
	fmt.Fprintln(out,
		`// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY`)
}

func writeTestMain(out io.Writer, hasModules, hasV23Tests bool) {
	switch {
	case hasModules && hasV23Tests:
		writeModulesAndV23TestMain(out)
	case hasModules:
		writeModulesTestMain(out)
	case hasV23Tests:
		writeV23TestMain(out)
	default:
		writeGoTestMain(out)
	}
}

var modulesSubprocess = `
	if modules.IsModulesProcess() {
		if err := modules.Dispatch(); err != nil {
			fmt.Fprintf(os.Stderr, "modules.Dispatch failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
`

// writeModulesTestMain writes out a TestMain appropriate for tests
// that have only modules.
func writeModulesTestMain(out io.Writer) {
	fmt.Fprint(out, `
func TestMain(m *testing.M) {
	testutil.Init()`+
		modulesSubprocess+
		`	os.Exit(m.Run())
}
`)
}

// writeModulesAndV23TestMain writes out a TestMain appropriate for tests
// that have both modules and v23 tests.
func writeModulesAndV23TestMain(out io.Writer) {
	fmt.Fprint(out, `
func TestMain(m *testing.M) {
	testutil.Init()`+
		modulesSubprocess+
		`	cleanup := v23tests.UseSharedBinDir()
	r := m.Run()
	cleanup()
	os.Exit(r)
}
`)
}

// writeV23TestMain writes out a TestMain appropriate for tests
// that have only v23 tests.
func writeV23TestMain(out io.Writer) {
	fmt.Fprint(out, `
func TestMain(m *testing.M) {
	testutil.Init()
	cleanup := v23tests.UseSharedBinDir()
	r := m.Run()
	cleanup()
	os.Exit(r)
}
`)
}

// writeGoTestMain writes out a TestMain appropriate for vanadium
// tests that use neither modules nor v23 tests.
func writeGoTestMain(out io.Writer) {
	fmt.Fprint(out, `
func TestMain(m *testing.M) {
	testutil.Init()
	os.Exit(m.Run())
}
`)
}

func writeModuleRegistration(out io.Writer, modules []moduleCommand) {
	for _, m := range modules {
		fmt.Fprintf(out, "\tmodules.RegisterChild(%q, `%s`, %s)\n", m.name, m.comment, m.name)
	}
}
