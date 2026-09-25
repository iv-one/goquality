package check

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/iv-one/goquality/internal/project"
)

// TestPresence reports how many packages have tests. Packages without any
// function declarations (types, constants, generated code) are not expected
// to have tests.
func TestPresence() Check {
	return checkFunc{name: "tests", category: Tests, weight: 0.05, run: func(_ context.Context, env *Env) Result {
		p := env.Project
		funcs := make(map[string]int)
		dirs := make(map[string]string)
		for _, f := range p.SourceFiles(false) {
			dirs[f.Package] = path.Dir(f.Rel)
			for _, d := range f.Syntax.Decls {
				if _, ok := d.(*ast.FuncDecl); ok {
					funcs[f.Package]++
				}
			}
		}
		var findings []Finding
		testable, tested := 0, 0
		for _, pkg := range p.Packages {
			if funcs[pkg] == 0 {
				continue
			}
			testable++
			if p.Tested[pkg] {
				tested++
			} else {
				findings = append(findings, Finding{File: dirs[pkg], Message: "package " + pkg + " has no tests"})
			}
		}
		s := p.Stats()
		r := Result{
			Summary:  fmt.Sprint(s.Tests),
			Findings: findings,
			Metrics: []Metric{
				{Key: "packages_tested", Label: "packages tested", Value: fmt.Sprintf("%d/%d", tested, testable)},
				{Key: "test_files", Label: "test files", Value: s.TestFiles},
			},
		}
		if s.Benchmarks > 0 {
			r.Metrics = append(r.Metrics, Metric{Key: "benchmarks", Label: "benchmarks", Value: s.Benchmarks})
		}
		if s.FuzzTests > 0 {
			r.Metrics = append(r.Metrics, Metric{Key: "fuzz_tests", Label: "fuzz tests", Value: s.FuzzTests})
		}
		if s.Examples > 0 {
			r.Metrics = append(r.Metrics, Metric{Key: "examples", Label: "examples", Value: s.Examples})
		}
		if testable > 0 {
			r.Score = ptr(float64(tested) / float64(testable))
		}
		return r
	}}
}

// Coverage runs the project's tests with statement coverage. It is opt-in
// because it executes project code.
func Coverage() Check {
	return checkFunc{name: "coverage", category: Tests, weight: 0.10, run: func(ctx context.Context, env *Env) Result {
		if !env.Options.Coverage {
			return Result{Status: Skipped, Summary: "skipped (--cover)"}
		}
		return runCoverage(ctx, env)
	}}
}

func runCoverage(ctx context.Context, env *Env) Result {
	p := env.Project
	prof, err := os.CreateTemp("", "goquality-cover-*.out")
	if err != nil {
		return Result{Error: err.Error()}
	}
	defer func() { _ = os.Remove(prof.Name()) }()
	if err := prof.Close(); err != nil {
		return Result{Error: err.Error()}
	}

	args := append([]string{"test", "-json", "-vet=off", "-covermode=set", "-coverprofile=" + prof.Name()}, p.Patterns...)
	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // runs the project's own tests, by request
	cmd.Dir = p.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	passed, failed, findings := parseTestEvents(&stdout)
	profiles, err := cover.ParseProfiles(prof.Name())
	if err != nil || len(profiles) == 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && runErr != nil {
			msg = runErr.Error()
		}
		if msg == "" {
			msg = "no coverage profile produced"
		}
		return Result{Error: errorText(msg), Findings: findings}
	}

	covered, total := statements(p, profiles)
	if total == 0 {
		return Result{Status: Info, Summary: "no statements", Findings: findings}
	}
	score := float64(covered) / float64(total)
	return Result{
		Summary:  fmt.Sprintf("%.1f%%", score*100),
		Score:    &score,
		Findings: findings,
		Metrics: []Metric{
			{Key: "tests_passed", Label: "tests passed", Value: passed},
			{Key: "tests_failed", Label: "tests failed", Value: failed},
		},
	}
}

type testEvent struct {
	Action      string
	Package     string
	Test        string
	FailedBuild string
}

// parseTestEvents counts top-level test results in "go test -json" output
// and reports failures.
func parseTestEvents(r io.Reader) (passed, failed int, findings []Finding) {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var ev testEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.FailedBuild != "" && ev.Test == "":
			findings = append(findings, Finding{Message: "build failed: " + ev.FailedBuild})
		case ev.Test == "" || strings.Contains(ev.Test, "/"):
			// package summary or subtest
		case ev.Action == "pass":
			passed++
		case ev.Action == "fail":
			failed++
			findings = append(findings, Finding{Message: fmt.Sprintf("%s failed in %s", ev.Test, ev.Package)})
		}
	}
	if err := sc.Err(); err != nil {
		findings = append(findings, Finding{Message: "reading test output: " + err.Error()})
	}
	return passed, failed, findings
}

// statements counts covered and total statements, excluding generated files.
// A block profiled by several packages counts once.
func statements(p *project.Project, profiles []*cover.Profile) (covered, total int) {
	type blockKey struct {
		file                   string
		sl, sc, el, ec, nstmts int
	}
	blocks := make(map[blockKey]bool)
	for _, prof := range profiles {
		rel := strings.TrimPrefix(strings.TrimPrefix(prof.FileName, p.ModulePath), "/")
		if f := p.File(filepath.Join(p.Root, rel)); f != nil && f.Generated {
			continue
		}
		for _, b := range prof.Blocks {
			k := blockKey{prof.FileName, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt}
			blocks[k] = blocks[k] || b.Count > 0
		}
	}
	for k, hit := range blocks {
		total += k.nstmts
		if hit {
			covered += k.nstmts
		}
	}
	return covered, total
}

// errorText condenses multi-line tool output into a single short line.
func errorText(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, strings.TrimSuffix(line, ":"))
		}
	}
	s = strings.Join(parts, ": ")
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
