package project

import (
	"go/ast"
	"go/scanner"
	"go/token"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// Stats describes the shape of a project, independent of its quality.
type Stats struct {
	Module         string `json:"module,omitempty"`
	GoVersion      string `json:"go_version,omitempty"`
	Packages       int    `json:"packages"`
	TestedPackages int    `json:"tested_packages"`
	GoFiles        int    `json:"go_files"`
	TestFiles      int    `json:"test_files"`
	GeneratedFiles int    `json:"generated_files"`
	Lines          int    `json:"lines"`           // all lines in non-generated files
	CodeLines      int    `json:"code_lines"`      // non-blank, non-comment lines in non-test files
	TestCodeLines  int    `json:"test_code_lines"` // non-blank, non-comment lines in test files
	Functions      int    `json:"functions"`       // function and method declarations in non-test files
	Tests          int    `json:"tests"`
	Benchmarks     int    `json:"benchmarks"`
	FuzzTests      int    `json:"fuzz_tests"`
	Examples       int    `json:"examples"`

	// DirectDependencies and IndirectDependencies are read from go.mod.
	DirectDependencies   int `json:"direct_dependencies"`
	IndirectDependencies int `json:"indirect_dependencies"`
	// BuildDependencies is the number of modules providing packages that the
	// analyzed packages actually import, transitively.
	BuildDependencies int `json:"build_dependencies"`
}

// Stats returns descriptive statistics for the project.
func (p *Project) Stats() Stats {
	p.statsOnce.Do(func() { p.stats = p.computeStats() })
	return p.stats
}

func (p *Project) computeStats() Stats {
	s := Stats{
		Module:         p.ModulePath,
		Packages:       len(p.Packages),
		TestedPackages: len(p.Tested),
	}

	for _, f := range p.Files {
		if f.Generated {
			s.GeneratedFiles++
			continue
		}
		s.GoFiles++
		src, err := f.ReadFile()
		if err != nil {
			continue
		}
		lines, code := countLines(src)
		s.Lines += lines
		if f.Test {
			s.TestFiles++
			s.TestCodeLines += code
			countTests(f.Syntax, &s)
		} else {
			s.CodeLines += code
			for _, d := range f.Syntax.Decls {
				if _, ok := d.(*ast.FuncDecl); ok {
					s.Functions++
				}
			}
		}
	}

	if p.GoMod != "" {
		if data, err := os.ReadFile(p.GoMod); err == nil {
			if mf, err := modfile.ParseLax(p.GoMod, data, nil); err == nil {
				if mf.Go != nil {
					s.GoVersion = mf.Go.Version
				}
				for _, r := range mf.Require {
					if r.Indirect {
						s.IndirectDependencies++
					} else {
						s.DirectDependencies++
					}
				}
			}
		}
	}

	mods := make(map[string]bool)
	packages.Visit(p.Roots, nil, func(pkg *packages.Package) {
		if m := pkg.Module; m != nil && !m.Main {
			mods[m.Path] = true
		}
	})
	s.BuildDependencies = len(mods)
	return s
}

// countLines returns the total number of lines in src and the number of
// lines that contain at least one non-comment token.
func countLines(src []byte) (total, code int) {
	if len(src) == 0 {
		return 0, 0
	}
	total = strings.Count(string(src), "\n")
	if src[len(src)-1] != '\n' {
		total++
	}

	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	var sc scanner.Scanner
	sc.Init(file, src, nil, 0)

	seen := make(map[int]bool)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue // automatically inserted
		}
		start := file.Line(pos)
		end := start + strings.Count(lit, "\n")
		for l := start; l <= end; l++ {
			seen[l] = true
		}
	}
	return total, len(seen)
}

// countTests counts test, benchmark, fuzz and example functions using the
// same naming rules as "go test".
func countTests(f *ast.File, s *Stats) {
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		name := fn.Name.Name
		switch {
		case isTestName(name, "Test"):
			s.Tests++
		case isTestName(name, "Benchmark"):
			s.Benchmarks++
		case isTestName(name, "Fuzz"):
			s.FuzzTests++
		case isTestName(name, "Example"):
			s.Examples++
		}
	}
}

func isTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}
