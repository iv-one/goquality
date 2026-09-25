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
		Grade:  check.GradeA,
		Score:  85,
		Issues: 1,
		Checks: []check.Result{{
			Name: "errcheck", Label: "errcheck", Category: check.Correctness, Status: check.Warn, Summary: "1 issue",
			Findings: []check.Finding{{File: "main.go", Line: 3, Column: 2, Message: "unchecked error"}},
		}},
	}
	out := Text(rep, TextOptions{Verbose: true})
	for _, want := range []string{
		"Grade .................................. A\n",
		"Correctness\n  errcheck ....................... 1 issue\n",
		"      main.go:3:2 unchecked error\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
