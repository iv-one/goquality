// Package check implements goquality's quality checks and the runner that
// combines their results into a single weighted score.
package check

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/iv-one/goquality/internal/project"
)

// Category groups checks in the report.
type Category string

// Report sections, in display order.
const (
	Correctness     Category = "correctness"
	Maintainability Category = "maintainability"
	Tests           Category = "tests"
	Security        Category = "security"
)

// Categories lists all categories in display order.
var Categories = []Category{Correctness, Maintainability, Tests, Security}

// Status is the outcome of a check.
type Status string

// Check outcomes.
const (
	Pass    Status = "pass"
	Warn    Status = "warn" // has findings
	Info    Status = "info" // descriptive only, not pass/fail
	Skipped Status = "skipped"
	Failed  Status = "error" // the check could not run
)

// Check is a single quality signal.
type Check interface {
	Name() string
	Category() Category
	// Weight is the check's share of the overall score. Checks that are
	// skipped, fail to run or report no score do not count.
	Weight() float64
	Run(ctx context.Context, env *Env) Result
}

// Finding is a single issue reported by a check.
type Finding struct {
	File     string `json:"file,omitempty"` // relative to the project root
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Severity string `json:"severity,omitempty"` // as defined by the underlying tool
	Message  string `json:"message"`
}

// Metric is a labeled value displayed with a check.
type Metric struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value any    `json:"value"`
}

// Result is the outcome of running a check.
type Result struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"` // display label; defaults to Name
	Category Category  `json:"category"`
	Status   Status    `json:"status"`
	Summary  string    `json:"summary"` // short display value, e.g. "PASS" or "3 issues"
	Weight   float64   `json:"weight"`
	Score    *float64  `json:"score,omitempty"` // 0..1; nil when not scored
	Metrics  []Metric  `json:"metrics,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
	Note     string    `json:"note,omitempty"`  // extra context, e.g. partial failures
	Error    string    `json:"error,omitempty"` // why the check could not run
	Duration float64   `json:"duration_seconds"`
}

// Options configure a run.
type Options struct {
	// CyclomaticThreshold is the complexity above which a function is
	// reported. Defaults to 15, as in Go Report Card.
	CyclomaticThreshold int
	// Coverage enables running the project's tests with coverage.
	Coverage bool
}

// Env is shared state available to checks during a run.
type Env struct {
	Project *project.Project
	Options Options

	analysis analysisRun
	suppress suppressions
}

// Report is the combined result of a run.
type Report struct {
	Stats    project.Stats `json:"project"`
	Grade    Grade         `json:"grade"`
	Score    float64       `json:"score"` // 0..100
	Issues   int           `json:"issues"`
	Checks   []Result      `json:"checks"`
	Duration float64       `json:"duration_seconds"`
}

// Run runs checks concurrently against a loaded project.
func Run(ctx context.Context, p *project.Project, checks []Check, opts Options) Report {
	start := time.Now()
	if opts.CyclomaticThreshold <= 0 {
		opts.CyclomaticThreshold = 15
	}
	env := &Env{Project: p, Options: opts}
	env.suppress.init()
	for _, c := range checks {
		if ac, ok := c.(interface{ analyzerSet() analyzerSet }); ok {
			env.analysis.register(ac.analyzerSet())
		}
	}

	results := make([]Result, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Go(func() {
			t := time.Now()
			r := c.Run(ctx, env)
			r.Name = c.Name()
			if r.Label == "" {
				r.Label = r.Name
			}
			r.Category = c.Category()
			r.Weight = c.Weight()
			finalize(&r)
			r.Duration = time.Since(t).Seconds()
			results[i] = r
		})
	}
	wg.Wait()

	rep := Report{Stats: p.Stats(), Checks: results}
	var total, weight float64
	for _, r := range results {
		rep.Issues += len(r.Findings)
		if r.Score != nil && r.Weight > 0 {
			total += *r.Score * r.Weight
			weight += r.Weight
		}
	}
	if weight > 0 {
		rep.Score = total / weight * 100
	}
	rep.Grade = GradeFromPercentage(rep.Score)
	rep.Duration = time.Since(start).Seconds()
	return rep
}

// finalize fills in defaults shared by all checks: sorted findings, a status
// and a summary.
func finalize(r *Result) {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	switch {
	case r.Error != "":
		r.Status, r.Score = Failed, nil
		if r.Summary == "" {
			r.Summary = "error"
		}
	case r.Status == Skipped:
		r.Score = nil
		if r.Summary == "" {
			r.Summary = "skipped"
		}
	case r.Status == "":
		if len(r.Findings) == 0 {
			r.Status = Pass
		} else {
			r.Status = Warn
		}
	}
	if r.Summary == "" {
		r.Summary = issuesSummary(len(r.Findings))
	}
}

func issuesSummary(n int) string {
	switch n {
	case 0:
		return "PASS"
	case 1:
		return "1 issue"
	default:
		return strconv.Itoa(n) + " issues"
	}
}

// fileScore is Go Report Card's scoring rule: the share of files without
// any finding.
func fileScore(files int, findings []Finding) *float64 {
	if files == 0 {
		return nil
	}
	bad := make(map[string]bool)
	for _, f := range findings {
		bad[f.File] = true
	}
	s := float64(files-len(bad)) / float64(files)
	if s < 0 {
		s = 0
	}
	return &s
}

func ptr(f float64) *float64 { return &f }
