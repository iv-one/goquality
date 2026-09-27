package report

import (
	"strings"
	"testing"

	"github.com/iv-one/goquality/internal/check"
)

func TestNum(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 18420: "18,420", 1234567: "1,234,567"} {
		if got := num(n); got != want {
			t.Errorf("num(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestText(t *testing.T) {
	rep := check.Report{
		Grade:     check.GradeA,
		Score:     85,
		Issues:    1,
		NextSteps: []check.Step{{Check: "errcheck", Label: "errcheck", Gain: 6, Issues: 1, Files: 1, Hint: "Handle it."}},
		Checks: []check.Result{{
			Name: "errcheck", Label: "errcheck", Category: check.Correctness, Status: check.Warn, Summary: "1 issue",
			Findings: []check.Finding{{File: "main.go", Line: 3, Column: 2, Message: "unchecked error"}},
		}},
	}
	if out := Text(rep, TextOptions{Verbose: true}); strings.Contains(out, "Next steps") {
		t.Errorf("next steps shown without NextSteps:\n%s", out)
	}
	out := Text(rep, TextOptions{Verbose: true, NextSteps: true})
	for _, want := range []string{
		"Grade .................................. A\n",
		"Correctness\n  errcheck ....................... 1 issue\n",
		"      main.go:3:2 unchecked error\n",
		"Next steps (A+ needs > 90%: fix 1)\n  1.  +6.0%  errcheck  1 issue\n             Handle it.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTextBlockers(t *testing.T) {
	rep := check.Report{
		Version:       "v0.5.0",
		Grade:         check.GradeB,
		Score:         80,
		Blockers:      1,
		UncappedScore: 97.3,
		Checks: []check.Result{{
			Name: "gosec", Label: "security findings", Category: check.Security, Status: check.Warn, Summary: "1",
			Findings: []check.Finding{{File: "tls.go", Line: 6, Rule: "G402", Severity: "HIGH", Blocker: true, Message: "G402: TLS InsecureSkipVerify set true."}},
		}},
	}
	out := Text(rep, TextOptions{Verbose: true})
	for _, want := range []string{
		"Score .............................. 80.0%\n  capped at 80% by 1 blocker (97.3% without it)\n",
		"      tls.go:6 [HIGH, blocker] G402: TLS InsecureSkipVerify set true.\n",
		"  goquality version ............... v0.5.0\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
