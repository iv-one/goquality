// Package project loads a local Go project once and exposes the parsed,
// type-checked packages and source files that every check works from.
package project

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// File is a Go source file that belongs to the analyzed project.
type File struct {
	Path      string // absolute path
	Rel       string // path relative to the project root, slash-separated
	Package   string // import path of the (non-test) package it belongs to
	Test      bool   // _test.go file
	Generated bool   // carries a "Code generated ... DO NOT EDIT." header
	Syntax    *ast.File
}

// Project is a loaded Go project.
type Project struct {
	Root       string   // absolute directory the analysis was started from
	Patterns   []string // package patterns, relative to Root
	ModulePath string
	ModuleDir  string
	GoMod      string // path to go.mod, empty outside module mode
	Fset       *token.FileSet

	// Roots are the packages to analyze; see selectRoots.
	Roots []*packages.Package

	// Packages holds the import paths of the project's packages, excluding
	// test-only packages.
	Packages []string

	// Tested holds the import paths of packages that have test files.
	Tested map[string]bool

	// Files are the project's Go files, sorted by Rel. Generated files are
	// included and flagged.
	Files []*File

	byPath    map[string]*File
	statsOnce sync.Once
	stats     Stats
}

// Load loads the packages matching patterns (default "./...") from dir.
func Load(ctx context.Context, dir string, patterns []string) (*Project, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	if !inModule(root) {
		return nil, fmt.Errorf("no go.mod found in %s or any parent directory", root)
	}

	fset := token.NewFileSet()
	cfg := &packages.Config{
		Context: ctx,
		Dir:     root,
		Fset:    fset,
		Tests:   true,
		Mode:    packages.LoadAllSyntax | packages.NeedModule | packages.NeedForTest,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	p := &Project{
		Root:     root,
		Patterns: patterns,
		Fset:     fset,
		Tested:   make(map[string]bool),
		byPath:   make(map[string]*File),
	}
	seenPkg := make(map[string]bool)
	for _, pkg := range selectRoots(pkgs) {
		p.Roots = append(p.Roots, pkg)
		owner := owner(pkg)
		if !seenPkg[owner] {
			seenPkg[owner] = true
			p.Packages = append(p.Packages, owner)
		}
		if p.ModulePath == "" && pkg.Module != nil && pkg.Module.Main {
			p.ModulePath = pkg.Module.Path
			p.ModuleDir = pkg.Module.Dir
			p.GoMod = pkg.Module.GoMod
		}
		p.addFiles(pkg)
	}
	sort.Strings(p.Packages)
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Rel < p.Files[j].Rel })

	if len(p.Files) == 0 {
		return nil, noFilesError(root, pkgs)
	}
	return p, nil
}

// selectRoots picks the packages to analyze: each package in its test
// variant when one exists (a superset of the plain package), plus external
// _test packages, without synthesized test mains.
func selectRoots(pkgs []*packages.Package) []*packages.Package {
	hasTestVariant := make(map[string]bool)
	for _, pkg := range pkgs {
		if pkg.ForTest != "" && pkg.PkgPath == pkg.ForTest {
			hasTestVariant[pkg.PkgPath] = true
		}
	}
	var roots []*packages.Package
	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.PkgPath, ".test") || (pkg.ForTest == "" && hasTestVariant[pkg.PkgPath]) {
			continue
		}
		roots = append(roots, pkg)
	}
	return roots
}

// owner returns the import path of the package a (possibly test) package
// belongs to.
func owner(pkg *packages.Package) string {
	if pkg.ForTest != "" {
		return pkg.ForTest
	}
	return pkg.PkgPath
}

func (p *Project) addFiles(pkg *packages.Package) {
	for _, f := range pkg.Syntax {
		path := p.Fset.File(f.Pos()).Name()
		if p.byPath[path] != nil {
			continue
		}
		rel, err := filepath.Rel(p.Root, path)
		if err != nil || strings.HasPrefix(rel, "..") || !strings.HasSuffix(path, ".go") {
			continue // e.g. cgo output in the build cache
		}
		file := &File{
			Path:      path,
			Rel:       filepath.ToSlash(rel),
			Package:   owner(pkg),
			Test:      strings.HasSuffix(path, "_test.go"),
			Generated: ast.IsGenerated(f),
			Syntax:    f,
		}
		if file.Test {
			p.Tested[file.Package] = true
		}
		p.byPath[path] = file
		p.Files = append(p.Files, file)
	}
}

func noFilesError(root string, pkgs []*packages.Package) error {
	var msgs []string
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			msgs = append(msgs, e.Error())
		}
	})
	if len(msgs) > 0 {
		return errors.New(strings.Join(msgs, "\n"))
	}
	return fmt.Errorf("no Go files found in %s", root)
}

// File returns the project file at the given absolute path, or nil if the
// path is not part of the project.
func (p *Project) File(path string) *File {
	return p.byPath[path]
}

// SourceFiles returns the non-generated project files, optionally including
// tests.
func (p *Project) SourceFiles(tests bool) []*File {
	var out []*File
	for _, f := range p.Files {
		if f.Generated || (f.Test && !tests) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// ReadFile reads a project file's contents.
func (f *File) ReadFile() ([]byte, error) {
	return os.ReadFile(f.Path) //nolint:gosec // reading project sources is the point
}

// inModule reports whether dir is inside a Go module or workspace.
func inModule(dir string) bool {
	if os.Getenv("GO111MODULE") == "off" {
		return true
	}
	for {
		for _, name := range []string{"go.mod", "go.work"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
