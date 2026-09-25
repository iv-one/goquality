package check

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/iv-one/goquality/internal/project"
)

func TestRun(t *testing.T) {
	p, err := project.Load(context.Background(), "../testdata/sample", nil)
	if err != nil {
		t.Fatal(err)
	}
	// govulncheck needs the network; see TestGovulncheck.
	checks := Filter(All(), nil, map[string]bool{"govulncheck": true}, true)
	rep := Run(context.Background(), p, checks, Options{CyclomaticThreshold: 4, Coverage: true})

	want := map[string][]string{
		"build":       nil,
		"govet":       {"lib/lib_test.go:14 tests", "main.go:15 printf"},
		"staticcheck": {"lib/lib.go:12 SA6005", "main.go:15 SA5009"},
		"errcheck":    {"main.go:12"}, // lines 17 and 19 are suppressed by //nolint
		"ineffassign": {"main.go:13"},
		"gofmt":       {"lib/ugly.go:4"},
		"complexity":  {"lib/lib.go:16"},
		"misspell":    {"main.go:20"},
		"license":     {":0"},
		"tests":       {".:0"},
		"coverage":    nil,
		"gosec":       {"main.go:4 G501", "main.go:16 G401"},
	}
	for _, r := range rep.Checks {
		if r.Error != "" {
			t.Errorf("%s: unexpected error %q", r.Name, r.Error)
		}
		var got []string
		for _, f := range r.Findings {
			s := fmt.Sprintf("%s:%d", f.File, f.Line)
			if f.Rule != "" {
				s += " " + f.Rule
			}
			got = append(got, s)
		}
		if !slices.Equal(got, want[r.Name]) {
			t.Errorf("%s findings = %q, want %q", r.Name, got, want[r.Name])
		}
	}

	byName := make(map[string]Result)
	for _, r := range rep.Checks {
		byName[r.Name] = r
	}
	if got := byName["staticcheck"].Findings[0].Fix; got != "replace with strings.EqualFold" {
		t.Errorf("SA6005 fix = %q", got)
	}
	if got := byName["gofmt"].Findings[0].Fix; got != "gofmt -w lib/ugly.go" {
		t.Errorf("gofmt fix = %q", got)
	}
	if s := byName["gofmt"].Score; s == nil || *s != 0.75 {
		t.Errorf("gofmt score = %v, want 0.75", s)
	}
	// Only lib.Add is exercised. The exact ratio depends on the Go version,
	// which changed how packages without tests appear in coverage profiles.
	if s := byName["coverage"].Score; s == nil || *s <= 0 || *s >= 0.1 {
		t.Errorf("coverage score = %v, want between 0 and 0.1", s)
	}
	if rep.Issues != 13 {
		t.Errorf("Issues = %d, want 13", rep.Issues)
	}
	if rep.Score <= 0 || rep.Score >= 100 {
		t.Errorf("Score = %v, want between 0 and 100", rep.Score)
	}
}

func TestGovulncheck(t *testing.T) {
	if testing.Short() {
		t.Skip("needs network access to vuln.go.dev")
	}
	p, err := project.Load(context.Background(), "../testdata/sample", nil)
	if err != nil {
		t.Fatal(err)
	}
	rep := Run(context.Background(), p, []Check{Govulncheck()}, Options{})
	r := rep.Checks[0]
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if r.Summary != "0" {
		t.Errorf("summary = %q, want 0", r.Summary)
	}
}

func TestGradeFromPercentage(t *testing.T) {
	tests := map[float64]Grade{100: GradeAPlus, 90.1: GradeAPlus, 90: GradeA, 75: GradeB, 65: GradeC, 55: GradeD, 45: GradeE, 40: GradeF, 0: GradeF}
	for pct, want := range tests {
		if got := GradeFromPercentage(pct); got != want {
			t.Errorf("GradeFromPercentage(%v) = %s, want %s", pct, got, want)
		}
	}
}

func TestDirectiveMatches(t *testing.T) {
	tests := []struct {
		d           directive
		check, rule string
		want        bool
	}{
		{nil, "errcheck", "", true},
		{directive{"errcheck"}, "errcheck", "", true},
		{directive{"govet"}, "govet", "printf", true},
		{directive{"printf"}, "govet", "printf", true},
		{directive{"SA6005"}, "staticcheck", "SA6005", true},
		{directive{"gosimple"}, "staticcheck", "S1000", true},
		{directive{"errcheck"}, "gosec", "G104", false},
	}
	for _, tt := range tests {
		if got := tt.d.matches(tt.check, tt.rule); got != tt.want {
			t.Errorf("%q.matches(%q, %q) = %v, want %v", tt.d, tt.check, tt.rule, got, tt.want)
		}
	}
}

func TestVulnFix(t *testing.T) {
	tests := []struct {
		v    vuln
		want string
	}{
		{vuln{module: "golang.org/x/text", fixed: "v0.3.8"}, "go get golang.org/x/text@v0.3.8"},
		{vuln{module: "stdlib", fixed: "v1.26.1"}, "build with Go 1.26.1 or later"},
		{vuln{module: "example.com/m"}, ""},
	}
	for _, tt := range tests {
		if got := vulnFix(&tt.v); got != tt.want {
			t.Errorf("vulnFix(%+v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}
