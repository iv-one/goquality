package check

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/golangci/misspell"
	"golang.org/x/tools/go/packages"
)

// checkFunc adapts a function to the Check interface.
type checkFunc struct {
	name     string
	category Category
	weight   float64
	run      func(ctx context.Context, env *Env) Result
}

func (c checkFunc) Name() string                             { return c.name }
func (c checkFunc) Category() Category                       { return c.category }
func (c checkFunc) Weight() float64                          { return c.weight }
func (c checkFunc) Run(ctx context.Context, env *Env) Result { return c.run(ctx, env) }

// Build reports packages that fail to load or type-check. Other checks can
// only partially analyze such packages.
func Build() Check {
	return checkFunc{name: "build", category: Correctness, weight: 0.10, run: func(_ context.Context, env *Env) Result {
		var findings []Finding
		broken := make(map[string]bool)
		seen := make(map[string]bool)
		for _, pkg := range env.Project.Roots {
			for _, e := range pkg.Errors {
				owner := pkg.PkgPath
				if pkg.ForTest != "" {
					owner = pkg.ForTest
				}
				broken[owner] = true
				f := packageError(env, e)
				key := fmt.Sprint(f)
				if !seen[key] {
					seen[key] = true
					findings = append(findings, f)
				}
			}
		}
		n := len(env.Project.Packages)
		var score *float64
		if n > 0 {
			score = ptr(float64(n-len(broken)) / float64(n))
		}
		return Result{Findings: findings, Score: score}
	}}
}

func packageError(env *Env, e packages.Error) Finding {
	f := Finding{Message: e.Msg}
	// Pos is "file:line:col", "file:line" or empty.
	parts := strings.Split(e.Pos, ":")
	if len(parts) >= 2 {
		if rel, err := filepath.Rel(env.Project.Root, parts[0]); err == nil && !strings.HasPrefix(rel, "..") {
			f.File = filepath.ToSlash(rel)
		} else {
			f.File = parts[0]
		}
		f.Line, _ = strconv.Atoi(parts[1])
		if len(parts) >= 3 {
			f.Column, _ = strconv.Atoi(parts[2])
		}
	}
	return f
}

// GoFmt reports files whose formatting differs from gofmt's.
func GoFmt() Check {
	return checkFunc{name: "gofmt", category: Maintainability, weight: 0.15, run: func(_ context.Context, env *Env) Result {
		files := env.Project.SourceFiles(true)
		var findings []Finding
		for _, f := range files {
			src, err := f.ReadFile()
			if err != nil {
				continue
			}
			out, err := format.Source(src)
			if err != nil || bytes.Equal(src, out) {
				continue
			}
			findings = append(findings, Finding{File: f.Rel, Line: firstDiffLine(src, out), Message: "file is not gofmt-ed", Fix: "gofmt -w " + f.Rel})
		}
		score := fileScore(len(files), findings)
		r := Result{Findings: findings, Score: score}
		if score != nil {
			r.Summary = percent(*score)
		}
		return r
	}}
}

func firstDiffLine(a, b []byte) int {
	line := 1
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return line
		}
		if a[i] == '\n' {
			line++
		}
	}
	return line
}

// Complexity measures cyclomatic complexity of non-test functions with
// gocyclo.
func Complexity() Check {
	return checkFunc{name: "complexity", category: Maintainability, weight: 0.10, run: func(_ context.Context, env *Env) Result {
		threshold := env.Options.CyclomaticThreshold
		var stats gocyclo.Stats
		for _, f := range env.Project.SourceFiles(false) {
			stats = gocyclo.AnalyzeASTFile(f.Syntax, env.Project.Fset, stats)
		}
		if len(stats) == 0 {
			return Result{Status: Info, Summary: "no functions"}
		}

		var findings []Finding
		maxStat := stats[0]
		for _, s := range stats {
			if s.Complexity > maxStat.Complexity {
				maxStat = s
			}
			if s.Complexity > threshold {
				file := s.Pos.Filename
				if pf := env.Project.File(file); pf != nil {
					file = pf.Rel
				}
				findings = append(findings, Finding{
					File:    file,
					Line:    s.Pos.Line,
					Column:  s.Pos.Column,
					Message: fmt.Sprintf("cyclomatic complexity %d of %s.%s is high (> %d)", s.Complexity, s.PkgName, s.FuncName, threshold),
				})
			}
		}
		findings = env.filter("complexity", findings)
		score := float64(len(stats)-len(findings)) / float64(len(stats))
		return Result{
			Summary:  percent(score),
			Score:    &score,
			Findings: findings,
			Metrics: []Metric{
				{Key: "average", Label: "average complexity", Value: round1(stats.AverageComplexity())},
				{Key: "max", Label: "max complexity", Value: maxStat.Complexity},
				{Key: "max_function", Label: "most complex", Value: maxStat.PkgName + "." + maxStat.FuncName},
				{Key: "over_threshold", Label: fmt.Sprintf("functions > %d", threshold), Value: len(findings)},
			},
		}
	}}
}

// Misspell reports commonly misspelled English words in comments and strings.
func Misspell() Check {
	return checkFunc{name: "misspell", category: Maintainability, weight: 0.02, run: func(_ context.Context, env *Env) Result {
		r := misspell.New()
		files := env.Project.SourceFiles(true)
		var findings []Finding
		for _, f := range files {
			src, err := f.ReadFile()
			if err != nil {
				continue
			}
			_, diffs := r.ReplaceGo(string(src))
			for _, d := range diffs {
				findings = append(findings, Finding{
					File:    f.Rel,
					Line:    d.Line,
					Column:  d.Column + 1,
					Message: fmt.Sprintf("%q is a misspelling of %q", d.Original, d.Corrected),
				})
			}
		}
		findings = env.filter("misspell", findings)
		return Result{Findings: findings, Score: fileScore(len(files), findings)}
	}}
}

// licenseNames are file name prefixes recognized as a license, from
// github.com/ryanuber/go-license and client9 via Go Report Card.
var licenseNames = []string{"license", "licence", "copying", "copyright", "unlicense", "copyleft"}

// License checks for a license file at the module root.
func License() Check {
	return checkFunc{name: "license", category: Maintainability, weight: 0.03, run: func(_ context.Context, env *Env) Result {
		dir := env.Project.ModuleDir
		if dir == "" {
			dir = env.Project.Root
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return Result{Error: err.Error()}
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if e.IsDir() || filepath.Ext(name) == ".go" {
				continue
			}
			for _, prefix := range licenseNames {
				if strings.HasPrefix(name, prefix) {
					return Result{Score: ptr(1), Summary: e.Name()}
				}
			}
		}
		return Result{
			Score:    ptr(0),
			Findings: []Finding{{Message: "no license file found at the module root (see https://choosealicense.com)"}},
			Summary:  "missing",
		}
	}}
}

func percent(f float64) string {
	return fmt.Sprintf("%.0f%%", f*100)
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
