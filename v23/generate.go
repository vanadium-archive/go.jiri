// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/x/devtools/internal/collect"
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

1. v.io/veyron/test/modules requires the use of an explicit
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

	ArgsName: "[packages]",
	ArgsLong: "list of go packages"}

const (
	defaultV23TestPrefix   = "v23"
	externalSuffix         = "_test.go"
	internalSuffix         = "_internal_test.go"
	traceTransitiveImports = false
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
	packages, err := goutil.List(ctx, args...)
	if err != nil {
		return env.UsageErrorf("failed to list %s: %s", args, err)
	}

	cleanup, err := configureBuilder(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	if progressFlag {
		fmt.Fprintf(ctx.Stdout(), "test generate %v expands to %v\n", args, packages)
	}

	for _, pkg := range packages {
		bpkg, err := build.Import(pkg, ".", build.ImportMode(build.ImportComment))
		if err != nil {
			return env.UsageErrorf("failed to import %q: err: %s", pkg, err)
		}
		if err := generatePackage(ctx, bpkg); err != nil {
			return err
		}
	}
	return nil
}

// processFiles parses the specified files and returns the following:
// - a boolean indicating if the package already includes a TestMain
// - a list of modules command functions defined in these files
// - a list of the V23tests tests defined in these files
func processFiles(fset *token.FileSet, dir string, files []string) (bool, []moduleCommand, []string, error) {
	re := regexp.MustCompile(`V23Test(.*)`)
	modules := []moduleCommand{}
	hasTestMain := false
	v23Tests := []string{}
	for _, base := range files {
		file := filepath.Join(dir, base)
		// Ignore the files we are generating.
		if base == prefixFlag+externalSuffix || base == prefixFlag+internalSuffix {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return false, nil, nil, fmt.Errorf("error parsing %q: %s", file, err)
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
				modules = append(modules, moduleCommand{n, c})
			}

			// If this function is the testing TestMain then
			// keep track of the fact that we've seen it.
			if isTestMain(fn) {
				hasTestMain = true
			}

			name := fn.Name.String()
			if parts := re.FindStringSubmatch(name); len(parts) == 2 {
				v23Tests = append(v23Tests, parts[1])
			}
		}
	}
	return hasTestMain, modules, v23Tests, nil
}

// importCache keeps track of which files have been imported and parsed so
// that we only parse them once, rather than every time an import of them
// is encountered.
type importCache map[string]struct{}

func (c importCache) transitiveModules(pkg *build.Package, imports []string, fset *token.FileSet) (bool, error) {
	if _, present := c[pkg.ImportPath]; present {
		return false, nil
	}
	gorootPrefix := build.Default.GOROOT + string(filepath.Separator)
	for _, imported := range imports {
		// ignore cgo imports.
		if imported == "C" {
			continue
		}
		bpkg, err := build.Import(imported, ".", build.ImportMode(build.ImportComment))
		if err != nil {
			return false, fmt.Errorf("Import(%q) failed: %v", imported, err)
		}
		if filepath.HasPrefix(bpkg.Dir, gorootPrefix) {
			continue
		}
		if traceTransitiveImports {
			fmt.Fprintf(os.Stderr, "%s: %v\n", pkg.ImportPath, imported)
		}
		for _, base := range bpkg.GoFiles {
			file := filepath.Join(bpkg.Dir, base)
			f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
			if err != nil {
				return false, fmt.Errorf("ParseFile(%q): failed: %v", file, err)
			}
			localModulesName := findModulesLocalName(f.Imports)
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if len(localModulesName) > 0 &&
					hasRegisterChild(fn.Body, localModulesName) {
					return true, nil
				}
			}
		}
		if traceTransitiveImports {
			fmt.Fprintf(os.Stderr, "%s: %v\n", imported, bpkg.Imports)
		}
		usesModules, err := c.transitiveModules(bpkg, bpkg.Imports, fset)
		if err != nil {
			return false, err
		}
		if usesModules {
			return true, nil
		}
	}

	c[pkg.ImportPath] = struct{}{}
	return false, nil
}

func importsModules(imports []string) bool {
	for _, imp := range imports {
		if imp == "v.io/x/ref/test/modules" || imp == "v.io/x/ref/test/v23tests" {
			return true
		}
	}
	return false
}

func filteredImports(imports []string, pos map[string][]token.Position) []string {
	var ret []string
	for _, imp := range imports {
		// If the only importer of this package is one of the files we are regenerating,
		// then filter those imports out.
		if positions := pos[imp]; len(positions) == 1 {
			if fname := positions[0].Filename; fname == prefixFlag+externalSuffix || fname == prefixFlag+internalSuffix {
				continue
			}
		}
		ret = append(ret, imp)
	}
	return ret
}

func generatePackage(ctx *tool.Context, pkg *build.Package) error {
	fset := token.NewFileSet() // positions are relative to fset
	cache := importCache{}
	testImports := filteredImports(pkg.TestImports, pkg.TestImportPos)
	xtestImports := filteredImports(pkg.XTestImports, pkg.XTestImportPos)
	intTestUsesModules := importsModules(testImports)
	intTestMain, intModules, _, err := processFiles(fset, pkg.Dir, pkg.TestGoFiles)
	if err != nil {
		return err
	}
	extTestUsesModules := importsModules(xtestImports)
	extTestMain, extModules, v23Tests, err := processFiles(fset, pkg.Dir, pkg.XTestGoFiles)
	if err != nil {
		return err
	}

	// Don't bother with transitive checks if we don't actually call
	// modules from this test.
	intDepsDefineModules, extDepsDefineModules := false, false
	if intTestUsesModules {
		// Determine if we transitively import packages that define modules.
		imports := append([]string{}, pkg.Imports...)
		imports = append(imports, testImports...)
		intDepsDefineModules, err = cache.transitiveModules(pkg, imports, fset)
		if err != nil {
			return err
		}
	}

	// TODO(suharshs): Find a better way to fix this, maybe always generating TestMain
	// in the internal package could solve this?
	//
	// We need to clear the root package from the first transitiveModules call to prevent
	// the transitiveModules external package call from always returning true.
	delete(cache, pkg.ImportPath)

	if extTestUsesModules {
		// Determine if we transitively import packages that define modules.
		extDepsDefineModules, err = cache.transitiveModules(pkg, xtestImports, fset)
		if err != nil {
			return err
		}
	}

	needsIntTM := !intTestMain
	needsExtTM := !extTestMain

	hasV23Tests := len(v23Tests) > 0
	needIntFile := len(intModules) > 0
	needExtFile := len(extModules) > 0 || hasV23Tests

	// TestMain is special in that it can only occur once even across
	// internal and external test packages. If it doesn't occur
	// in either, we want to make sure we write it out in the internal
	// package.
	if (needsIntTM || needsExtTM) && !needIntFile && !needExtFile {
		needIntFile = true
		needsIntTM = true
		needsExtTM = false
	}

	extFile := filepath.Join(pkg.Dir, prefixFlag+externalSuffix)
	intFile := filepath.Join(pkg.Dir, prefixFlag+internalSuffix)

	if progressFlag {
		fmt.Fprintf(ctx.Stdout(), "Package: %s\n", pkg.ImportPath)
		if needIntFile {
			fmt.Fprintf(ctx.Stdout(), "Writing internal test file: %s\n", intFile)
		}
		if needExtFile {
			fmt.Fprintf(ctx.Stdout(), "Writing external test file: %s\n", extFile)
		}
		if hasV23Tests {
			fmt.Fprintf(ctx.Stdout(), "Number of V23Tests: %d\n", len(v23Tests))
		}
		if len(intModules) > 0 {
			fmt.Fprintf(ctx.Stdout(), "Number of internal module commands: %d\n", len(intModules))
		}
		if len(extModules) > 0 {
			fmt.Fprintf(ctx.Stdout(), "Number of external module commands: %d\n", len(extModules))
		}
		modulesSummary := func(s string, uses, imports bool) {
			switch {
			case uses && imports:
				fmt.Fprintf(ctx.Stdout(), "%s test uses modules and transitively imports them.\n", s)
			case uses && !imports:
				fmt.Fprintf(ctx.Stdout(), "%s test uses modules but does not transitively import them.\n", s)
			case !uses && imports:
				fmt.Fprintf(ctx.Stdout(), "%s test does not uses modules but transitively imports them.\n", s)
			case !uses && !imports:
				fmt.Fprintf(ctx.Stdout(), "%s test does not use modules and does not transitively imports them.\n", s)
			}
		}
		modulesSummary("Internal", intTestUsesModules, intDepsDefineModules)
		modulesSummary("External", extTestUsesModules, extDepsDefineModules)

		if !needIntFile && intDepsDefineModules && intTestUsesModules {
			fmt.Fprintf(ctx.Stdout(), "Internal tests imports and uses modules, but TestMain is being written to an external test file.\n")
		}
	}

	if needIntFile {
		if err := writeInternalFile(intFile, pkg.Name, needsIntTM || needsExtTM, intDepsDefineModules || extDepsDefineModules, hasV23Tests, intModules); err != nil {
			return err
		}
		needsExtTM = false
	}

	if needExtFile {
		if !needIntFile && intDepsDefineModules && intTestUsesModules {
			extDepsDefineModules = true
		}
		return writeExternalFile(extFile, pkg.Name, needsExtTM, extDepsDefineModules, extModules, v23Tests)
	}
	return nil
}

func findModulesLocalName(imports []*ast.ImportSpec) string {
	for _, i := range imports {
		if i.Path.Value == `"v.io/x/ref/test/modules"` {
			if i.Name != nil {
				return i.Name.Name
			}
			return "modules"
		}
	}
	return ""
}

func hasRegisterChild(body *ast.BlockStmt, importName string) bool {
	if body == nil {
		return false
	}
	for _, l := range body.List {
		expr, ok := l.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		fn, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		method := fn.Sel.Name
		pkg, ok := fn.X.(*ast.Ident)
		if !ok {
			continue
		}
		if pkg.Name == importName && method == "RegisterChild" {
			return true
		}
	}
	return false
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
func writeInternalFile(fileName string, packageName string, needsTestMain, needsModulesDispatch, hasV23Tests bool, modules []moduleCommand) (e error) {

	hasModules := len(modules) > 0
	needsModulesInTestMain := needsModulesDispatch || hasModules

	if !needsTestMain && !needsModulesInTestMain {
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
		if needsModulesInTestMain {
			fmt.Fprintln(out, `import "fmt"`)
		}
		fmt.Fprintln(out, `import "testing"`)
		fmt.Fprintln(out, `import "os"`)
		fmt.Fprintln(out)
		fmt.Fprintln(out, `import "v.io/x/ref/test"`)
		if needsModulesInTestMain {
			fmt.Fprintln(out, `import "v.io/x/ref/test/modules"`)
		}
		if hasV23Tests {
			fmt.Fprintln(out, `import "v.io/x/ref/test/v23tests"`)
		}
	}

	if hasModules {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "func init() {")
		writeModuleRegistration(out, modules)
		fmt.Fprintln(out, "}")
	}

	if needsTestMain {
		writeTestMain(out, needsModulesInTestMain, hasV23Tests)
	}

	return nil
}

// writeExternalFile write a generated test file that is outside the package.
// It can contain intgreation tests.
func writeExternalFile(fileName string, packageName string, needsTestMain, needsModulesDispatch bool, modules []moduleCommand, v23Tests []string) (e error) {

	hasV23Tests := len(v23Tests) > 0
	hasModules := len(modules) > 0
	needsModulesInTestMain := needsModulesDispatch || hasModules
	if !needsTestMain && !needsModulesInTestMain && !hasV23Tests {
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
	if needsTestMain && needsModulesInTestMain {
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
	if needsTestMain {
		fmt.Fprintln(out, `import "v.io/x/ref/test"`)
	}
	if needsModulesInTestMain {
		fmt.Fprintln(out, `import "v.io/x/ref/test/modules"`)
	}
	if hasV23Tests {
		fmt.Fprintln(out, `import "v.io/x/ref/test/v23tests"`)
	}

	if hasModules {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "func init() {")
		writeModuleRegistration(out, modules)
		fmt.Fprintln(out, "}")
	}

	if needsTestMain {
		writeTestMain(out, needsModulesInTestMain, hasV23Tests)
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

func writeTestMain(out io.Writer, needsModulesDispatch, hasV23Tests bool) {
	switch {
	case needsModulesDispatch && hasV23Tests:
		writeModulesAndV23TestMain(out)
	case needsModulesDispatch:
		writeModulesTestMain(out)
	case hasV23Tests:
		writeV23TestMain(out)
	default:
		writeGoTestMain(out)
	}
}

var modulesSubprocess = `
	if modules.IsModulesChildProcess() {
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
	test.Init()`+
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
	test.Init()`+
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
	test.Init()
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
	test.Init()
	os.Exit(m.Run())
}
`)
}

func writeModuleRegistration(out io.Writer, modules []moduleCommand) {
	for _, m := range modules {
		fmt.Fprintf(out, "\tmodules.RegisterChild(%q, `%s`, %s)\n", m.name, m.comment, m.name)
	}
}
