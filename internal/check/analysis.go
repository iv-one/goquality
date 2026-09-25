package check

import (
	"context"
	"fmt"
	"sync"

	"github.com/gordonklaus/ineffassign/pkg/ineffassign"
	"github.com/kisielk/errcheck/errcheck"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/analysis/suite/vet"
	"honnef.co/go/tools/analysis/lint"
	"honnef.co/go/tools/simple"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"
)

// analyzerSet is implemented by checks built on go/analysis. The runner
// registers every check's analyzers up front so that they all execute in a
// single pass over the loaded packages, sharing parsing, type information and
// SSA.
type analyzerSet []*analysis.Analyzer

type analysisRun struct {
	once      sync.Once
	analyzers []*analysis.Analyzer
	findings  map[*analysis.Analyzer][]Finding
	failed    map[*analysis.Analyzer]int // packages the analyzer could not run on
	err       error
}

func (a *analysisRun) register(set analyzerSet) {
	a.analyzers = append(a.analyzers, set...)
}

// diagnostics returns the findings of the given analyzers, running the shared
// analysis pass on first use.
func (e *Env) diagnostics(set analyzerSet) (findings []Finding, failed int, err error) {
	a := &e.analysis
	a.once.Do(func() { a.err = e.runAnalysis() })
	if a.err != nil {
		return nil, 0, a.err
	}
	for _, an := range set {
		findings = append(findings, a.findings[an]...)
		failed = max(failed, a.failed[an])
	}
	return findings, failed, nil
}

func (e *Env) runAnalysis() error {
	a := &e.analysis
	graph, err := checker.Analyze(a.analyzers, e.Project.Roots, nil)
	if err != nil {
		return err
	}
	a.findings = make(map[*analysis.Analyzer][]Finding)
	a.failed = make(map[*analysis.Analyzer]int)
	seen := make(map[string]bool)
	for _, act := range graph.Roots {
		if act.Err != nil {
			a.failed[act.Analyzer]++
			continue
		}
		for _, d := range act.Diagnostics {
			pos := e.Project.Fset.Position(d.Pos)
			f := e.Project.File(pos.Filename)
			if f == nil || f.Generated {
				continue
			}
			// Packages are analyzed together with their test variants, so
			// the same diagnostic can be reported more than once.
			key := fmt.Sprintf("%s:%d:%d:%s:%s", pos.Filename, pos.Line, pos.Column, act.Analyzer.Name, d.Message)
			if seen[key] {
				continue
			}
			seen[key] = true
			a.findings[act.Analyzer] = append(a.findings[act.Analyzer], Finding{
				File:    f.Rel,
				Line:    pos.Line,
				Column:  pos.Column,
				Rule:    act.Analyzer.Name,
				Message: d.Message,
			})
		}
	}
	return nil
}

// analyzerCheck is a check backed by one or more go/analysis analyzers.
type analyzerCheck struct {
	name      string
	label     string
	category  Category
	weight    float64
	analyzers analyzerSet
}

func (c analyzerCheck) Name() string             { return c.name }
func (c analyzerCheck) Category() Category       { return c.category }
func (c analyzerCheck) Weight() float64          { return c.weight }
func (c analyzerCheck) analyzerSet() analyzerSet { return c.analyzers }

func (c analyzerCheck) Run(_ context.Context, env *Env) Result {
	findings, failed, err := env.diagnostics(c.analyzers)
	if err != nil {
		return Result{Error: err.Error()}
	}
	findings = env.filter(c.name, findings)
	if len(c.analyzers) == 1 {
		for i := range findings {
			findings[i].Rule = ""
		}
	}
	r := Result{
		Label:    c.label,
		Findings: findings,
		Score:    fileScore(len(env.Project.SourceFiles(true)), findings),
	}
	if failed > 0 {
		r.Note = fmt.Sprintf("%d package(s) not analyzed due to build errors", failed)
	}
	return r
}

// GoVet runs the analyzers of "go vet".
func GoVet() Check {
	return analyzerCheck{name: "govet", label: "go vet", category: Correctness, weight: 0.20, analyzers: vet.Suite}
}

// Staticcheck runs staticcheck's default-enabled SA, S and ST analyzers.
func Staticcheck() Check {
	var set analyzerSet
	for _, group := range [][]*lint.Analyzer{staticcheck.Analyzers, simple.Analyzers, stylecheck.Analyzers} {
		for _, a := range group {
			if !a.Doc.NonDefault {
				set = append(set, a.Analyzer)
			}
		}
	}
	return analyzerCheck{name: "staticcheck", category: Correctness, weight: 0.15, analyzers: set}
}

// ErrCheck reports unchecked errors.
func ErrCheck() Check {
	return analyzerCheck{name: "errcheck", category: Correctness, weight: 0.10, analyzers: analyzerSet{errcheck.Analyzer}}
}

// IneffAssign reports assignments whose values are never used.
func IneffAssign() Check {
	return analyzerCheck{name: "ineffassign", category: Correctness, weight: 0.05, analyzers: analyzerSet{ineffassign.Analyzer}}
}
