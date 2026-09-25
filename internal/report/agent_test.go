package report

import (
	"strings"
	"testing"

	"github.com/iv-one/goquality/internal/check"
)

func TestAgent(t *testing.T) {
	rep := check.Report{
		Grade:      check.GradeA,
		Score:      85,
		Issues:     3,
		Suppressed: 1,
		NextSteps: []check.Step{
			{Check: "errcheck", Gain: 6, Issues: 2, Files: 1, Hint: "Handle it."},
			{Check: "gofmt", Gain: 1, Issues: 1, Files: 1},
		},
		Checks: []check.Result{
			{Name: "build", Status: check.Pass},
			{Name: "gofmt", Status: check.Warn, Findings: []check.Finding{{File: "b.go", Line: 1, Message: "file is not gofmt-ed", Fix: "gofmt -w b.go"}}},
			{Name: "errcheck", Status: check.Warn, Suppressed: 1, Findings: []check.Finding{
				{File: "a.go", Line: 9, Column: 2, Message: "unchecked error"},
				{File: "a.go", Line: 3, Column: 1, Message: "unchecked error"},
			}},
			{Name: "coverage", Status: check.Skipped, Summary: "skipped (--cover)"},
		},
	}
	out := Agent(rep, AgentOptions{Command: "goquality --agent", MaxFindings: 2})
	for _, want := range []string{
		"goquality: grade A (85.0%), 3 issues, 1 suppressed",
		"  fail: gofmt, errcheck\n  skip: coverage (--cover)\n  pass: build\n",
		"next steps, by score gain (A+ needs > 90%: fix 1)",
		"  1. errcheck +6.0% (2 issues) [1 suppressed]: Handle it.\n",
		"Re-check a single check: goquality --agent --only <check>",
		// Capped at 2: both errcheck findings (highest gain) win over gofmt.
		"findings (2 of 3; narrow with --only <check> or a package pattern):\na.go\n  3:1 errcheck: unchecked error\n  9:2 errcheck: unchecked error\n",
		"re-run goquality --agent to confirm",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "b.go") {
		t.Errorf("capped output lists gofmt finding:\n%s", out)
	}
}
