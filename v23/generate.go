// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"v.io/x/devtools/internal/goutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

var (
	prefixFlag   string
	progressFlag bool
)

func init() {
	cmdTestGenerate.Flags.StringVar(&prefixFlag, "prefix", defaultV23TestPrefix, "Specifies the prefix to use for generated files. Up to two files may generated, the defaults are v23_test.go and v23_internal_test.go, or <prefix>_test.go and <prefix>_internal_test.go.")
	cmdTestGenerate.Flags.BoolVar(&progressFlag, "progress", false, "Print verbose progress information.")
}

var cmdTestGenerate = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runTestGenerate),
	Name:   "generate",
	Short:  "Generate supporting code for v23 integration tests",
	Long: `
The generate command supports the vanadium integration test framework and unit
tests by generating go files that contain supporting code.  v23 test generate is
intended to be invoked via the 'go generate' mechanism and the resulting files
are to be checked in.

Integration tests are functions of the following form:

    func V23Test<x>(i *v23tests.T)

These functions are typically defined in 'external' *_test packages, to ensure
better isolation.  But they may also be defined directly in the 'internal' *
package.  The following helper functions will be generated:

    func TestV23<x>(t *testing.T) {
      v23tests.RunTest(t, V23Test<x>)
    }

In addition a TestMain function is generated, if it doesn't already exist.  Note
that Go requires that at most one TestMain function is defined across both the
internal and external test packages.

The generated TestMain performs common initialization, and also performs child
process dispatching for tests that use "v.io/veyron/test/modules".
`,

	ArgsName: "[packages]",
	ArgsLong: "list of go packages"}

const (
	defaultV23TestPrefix = "v23"
	externalSuffix       = "_test.go"
	internalSuffix       = "_internal_test.go"
)

func configureBuilder(ctx *tool.Context) (cleanup func(), err error) {
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain the Vanadium environment: %v", err)
	}
	prevGOPATH := build.Default.GOPATH
	cleanup = func() {
		build.Default.GOPATH = prevGOPATH
	}
	build.Default.GOPATH = env.Get("GOPATH")
	return cleanup, nil
}

func runTestGenerate(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	// Delete all files we're going to generate, to start with a clean slate.  We
	// do this first to avoid any issues where packages in the cache might include
	// the generated files.
	dirs, err := goutil.ListDirs(ctx, args...)
	if err != nil {
		return env.UsageErrorf("failed to list %v: %v", args, err)
	}
	for _, dir := range dirs {
		extFile := filepath.Join(dir, prefixFlag+externalSuffix)
		intFile := filepath.Join(dir, prefixFlag+internalSuffix)
		for _, f := range []string{extFile, intFile} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}

	// Now list the package paths and generate each one.
	paths, err := goutil.List(ctx, args...)
	if err != nil {
		return env.UsageErrorf("failed to list %v: %v", args, err)
	}

	cleanup, err := configureBuilder(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	if progressFlag {
		fmt.Fprintf(ctx.Stdout(), "test generate %v expands to %v\n", args, paths)
	}

	for _, path := range paths {
		if err := generatePackage(ctx, path); err != nil {
			return err
		}
	}
	return nil
}

// processFiles parses the specified files and returns the following:
// - a boolean indicating if the package already includes a TestMain
// - a list of the V23tests tests defined in these files
func processFiles(fset *token.FileSet, dir string, files []string) (bool, []string, error) {
	hasTestMain := false
	var v23Tests []string
	for _, base := range files {
		file := filepath.Join(dir, base)
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return false, nil, fmt.Errorf("error parsing %q: %s", file, err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := fn.Name.String()
			if name == "TestMain" {
				// TODO(toddw): Check TestMain signature?
				hasTestMain = true
			}
			if parts := regexpV23Test.FindStringSubmatch(name); len(parts) == 2 {
				v23Tests = append(v23Tests, parts[1])
			}
		}
	}
	return hasTestMain, v23Tests, nil
}

var regexpV23Test = regexp.MustCompile(`^V23Test(.*)$`)

func generatePackage(ctx *tool.Context, path string) error {
	pkg, err := importPackage(path)
	if err != nil {
		return err
	}
	hasTestFiles := len(pkg.TestGoFiles) > 0 || len(pkg.XTestGoFiles) > 0
	fset := token.NewFileSet()
	intHasTestMain, intV23Tests, err := processFiles(fset, pkg.Dir, pkg.TestGoFiles)
	if err != nil {
		return err
	}
	extHasTestMain, extV23Tests, err := processFiles(fset, pkg.Dir, pkg.XTestGoFiles)
	if err != nil {
		return err
	}
	needIntFile, needExtFile := len(intV23Tests) > 0, len(extV23Tests) > 0
	var tmOpts *testMainOpts
	if hasTestFiles && !intHasTestMain && !extHasTestMain {
		// TestMain may only occur once across the internal and external package.
		// If the internal package imports modules, we must put TestMain in the
		// internal file.  Otherwise we have a choice; we put TestMain in the
		// internal file if we're already generating it anyways, otherwise we put it
		// in the external file.
		tmOpts = new(testMainOpts)
		tmOpts.hasV23Tests = len(intV23Tests) > 0 || len(extV23Tests) > 0
		intModules, err := hasModulesImport(pkg.TestImports)
		switch {
		case err != nil:
			return err
		case intModules:
			tmOpts.hasModules = true
			needIntFile = true
		default:
			switch extModules, err := hasModulesImport(pkg.XTestImports); {
			case err != nil:
				return err
			case extModules:
				tmOpts.hasModules = true
				if !needIntFile {
					needExtFile = true
				}
			}
		}
		if !needIntFile && !needExtFile {
			// TestMain isn't already defined in either the internal or external
			// package, and neither modules nor v23 tests are defined.  We still want
			// to generate a TestMain for common test initialization, so we put it in
			// the internal file.
			needIntFile = true
		}
	}

	extFile := filepath.Join(pkg.Dir, prefixFlag+externalSuffix)
	intFile := filepath.Join(pkg.Dir, prefixFlag+internalSuffix)

	if progressFlag {
		fmt.Fprintf(ctx.Stdout(), "Package: %s\n", pkg.ImportPath)
		if needIntFile {
			fmt.Fprintf(ctx.Stdout(), "  Writing internal test file: %s\n", intFile)
			fmt.Fprintf(ctx.Stdout(), "    Internal v23 tests: %v\n", intV23Tests)
		}
		if needExtFile {
			fmt.Fprintf(ctx.Stdout(), "  Writing external test file: %s\n", extFile)
			fmt.Fprintf(ctx.Stdout(), "    External v23 tests: %v\n", extV23Tests)
		}
	}

	if needIntFile {
		if err := writeTestFile(intFile, pkg.Name, tmOpts, intV23Tests); err != nil {
			return err
		}
		tmOpts = nil // Never generate TestMain in both internal and external files.
	}
	if needExtFile {
		if err := writeTestFile(extFile, pkg.Name+"_test", tmOpts, extV23Tests); err != nil {
			return err
		}
	}
	return nil
}

var (
	pseudoC      = &build.Package{ImportPath: "C"}
	pseudoUnsafe = &build.Package{ImportPath: "unsafe"}
	pkgCache     = map[string]*build.Package{"C": pseudoC, "unsafe": pseudoUnsafe}
)

// importPackage loads and returns the package with the given package path.
func importPackage(path string) (*build.Package, error) {
	if p, ok := pkgCache[path]; ok {
		return p, nil
	}
	p, err := build.Import(path, "", build.AllowBinary)
	if err != nil {
		return nil, err
	}
	pkgCache[path] = p
	return p, nil
}

// hasModulesImport returns true iff "v.io/x/ref/test/modules" is listed
// directly in the imports packages, or transitively imported by those packages.
func hasModulesImport(imports []string) (bool, error) {
	for _, imp := range imports {
		if imp == "v.io/x/ref/test/modules" {
			return true, nil
		}
		pkg, err := importPackage(imp)
		if err != nil {
			return false, err
		}
		if result, err := hasModulesImport(pkg.Imports); result || err != nil {
			return result, err
		}
	}
	return false, nil
}

type testMainOpts struct {
	hasModules  bool
	hasV23Tests bool
}

// writeTestFile writes the generated test file.
func writeTestFile(fileName, pkgName string, tmOpts *testMainOpts, v23Tests []string) (e error) {
	// Generate the source file into buf.
	var buf bytes.Buffer
	fmt.Fprintf(&buf, `// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

package %s

import (
`, pkgName)
	if tmOpts != nil {
		fmt.Fprintln(&buf, `"os"`)
	}
	fmt.Fprintln(&buf, `"testing"`)
	fmt.Fprintln(&buf)
	if tmOpts != nil {
		fmt.Fprintln(&buf, `"v.io/x/ref/test"`)
	}
	if tmOpts != nil && tmOpts.hasModules {
		fmt.Fprintln(&buf, `"v.io/x/ref/test/modules"`)
	}
	if (tmOpts != nil && tmOpts.hasV23Tests) || len(v23Tests) > 0 {
		fmt.Fprintln(&buf, `"v.io/x/ref/test/v23tests"`)
	}
	fmt.Fprintln(&buf, `)`)
	if tmOpts != nil {
		tm := `
func TestMain(m *testing.M) {
	test.Init()`
		if tmOpts.hasModules {
			tm += `
	modules.DispatchAndExitIfChild()`
		}
		if tmOpts.hasV23Tests {
			tm += `
	cleanup := v23tests.UseSharedBinDir()
	r := m.Run()
	cleanup()
	os.Exit(r)`
		} else {
			tm += `
	os.Exit(m.Run())`
		}
		fmt.Fprint(&buf, tm+`
}
`)
	}
	for _, t := range v23Tests {
		fmt.Fprintf(&buf, `
func TestV23%s(t *testing.T) {
	v23tests.RunTest(t, V23Test%s)
}
`, t, t)
	}
	// Let gofmt remove extra imports and finicky formatting details.
	pretty, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fileName, pretty, 0660)
}
