package report

import (
	"strings"
	"testing"

	"github.com/iv-one/goquality/internal/check"
	"github.com/iv-one/goquality/internal/compare"
)

func comparison() compare.Comparison {
	s := func(f float64) *float64 { return &f }
	return compare.Comparison{
		Module:   "example.com/m",
		Baseline: compare.Side{Label: "origin/main", Commit: "6dbe3340a8c8", Score: 90, Suppressed: 1},
		Current:  compare.Side{Label: "working tree", Commit: "6dbe3340a8c8", Dirty: true, Score: 88.5, Suppressed: 2},
		Checks: []compare.Delta{
			{Check: "errcheck", Label: "errcheck", Status: compare.Fail, New: []check.Finding{
				{File: "a.go", Line: 12, Column: 2, Message: "unchecked error"},
			}},
			{Check: "coverage", Label: "coverage", Status: compare.Fail, Dropped: true, Base: s(0.784), Current: s(0.7805)},
			{Check: "gofmt", Label: "gofmt", Status: compare.Pass, Fixed: []check.Finding{{File: "b.go", Line: 1, Message: "file is not gofmt-ed"}}},
			{Check: "gosec", Label: "security findings", Status: compare.Skipped, Reason: "--no-security"},
		},
	}
}

func TestComparisonText(t *testing.T) {
	out := ComparisonText(comparison(), TextOptions{Verbose: true})
	for _, want := range []string{
		"Baseline ............. origin/main@6dbe334\n",
		"Current ..... working tree@6dbe334+changes\n",
		"Score ...................... 90.0% → 88.5%\n",
		"Suppressed ......................... 1 → 2\n",
		"  errcheck .......... FAIL (1 new finding)\n",
		"  coverage .......... FAIL (78.4% → 78.0%)\n",
		"  gofmt ................... PASS (1 fixed)\n",
		"  security findings .. not compared (--no-security)\n",
		"Regressions\n  errcheck: 1 new finding\n      a.go:12:2 unchecked error\n",
		"Fixed\n  gofmt:\n      b.go:1 file is not gofmt-ed\n",
		"FAILED: 2 checks regressed against origin/main\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestComparisonAgent(t *testing.T) {
	out := ComparisonAgent(comparison(), AgentOptions{Command: "goquality check --agent"})
	for _, want := range []string{
		"goquality check: FAIL, 2 checks regressed vs origin/main@6dbe334 | score 90.0% -> 88.5%, suppressed 1 -> 2 | example.com/m\n",
		"  errcheck: 1 new finding: Handle the returned errors",
		"    a.go:12:2 errcheck: unchecked error\n",
		"  coverage: 78.4% → 78.0%: Add tests",
		"fixed: gofmt 1\n",
		"not compared: gosec (--no-security)\n",
		"re-run goquality check --agent to confirm there are no regressions",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	c := comparison()
	c.Passed, c.Checks = true, nil
	out = ComparisonAgent(c, AgentOptions{})
	if !strings.HasPrefix(out, "goquality check: PASS, no regressions") || strings.Contains(out, "rules:") {
		t.Errorf("passing comparison:\n%s", out)
	}
}
