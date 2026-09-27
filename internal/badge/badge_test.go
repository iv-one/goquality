package badge

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/iv-one/goquality/internal/check"
)

func TestBadges(t *testing.T) {
	rep := check.Report{Score: 92.96, Grade: check.GradeAPlus}
	for _, tt := range []struct {
		name string
		svg  []byte
		want string
	}{
		{"score", Score(rep), "Go Quality: 93/100"},
		{"grade", Grade(rep), "Go Quality: A+"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := string(tt.svg)
			if !strings.Contains(s, `aria-label="`+tt.want+`"`) || !strings.Contains(s, "<title>"+tt.want+"</title>") {
				t.Errorf("badge does not say %q:\n%s", tt.want, s)
			}
			if !strings.Contains(s, `fill="#44cc11"`) {
				t.Errorf("A+ badge is not green:\n%s", s)
			}
			for _, bad := range []string{"<script", "href", "<style", "http://", "https://"} {
				if strings.Contains(strings.ReplaceAll(s, `xmlns="http://www.w3.org/2000/svg"`, ""), bad) {
					t.Errorf("badge contains %q", bad)
				}
			}
			if err := wellFormed(tt.svg); err != nil {
				t.Errorf("not well-formed XML: %v", err)
			}
		})
	}
	if !bytes.Equal(Score(rep), Score(rep)) {
		t.Error("output is not deterministic")
	}
}

func TestScoreRounds(t *testing.T) {
	// The report prints 99.95 as 100.0%; the badge must agree.
	for score, want := range map[float64]string{99.95: "100/100", 92.4: "92/100"} {
		if s := string(Score(check.Report{Score: score})); !strings.Contains(s, want) {
			t.Errorf("%v does not render as %s:\n%s", score, want, s)
		}
	}
}

func TestCoverage(t *testing.T) {
	score := func(f float64) *float64 { return &f }
	for _, tt := range []struct {
		name   string
		checks []check.Result
		want   string
		color  string
	}{
		{"measured", []check.Result{{Name: "errcheck", Score: score(0.1)}, {Name: "coverage", Status: check.Warn, Score: score(0.8549)}}, "Go coverage: 85%", Color(check.GradeA)},
		{"rounds", []check.Result{{Name: "coverage", Status: check.Pass, Score: score(0.9995)}}, "Go coverage: 100%", Color(check.GradeAPlus)},
		{"low", []check.Result{{Name: "coverage", Status: check.Pass, Score: score(0.12)}}, "Go coverage: 12%", Color(check.GradeF)},
		{"skipped", []check.Result{{Name: "coverage", Status: check.Skipped, Summary: "skipped (--cover)"}}, "Go coverage: n/a", Unknown},
		{"error", []check.Result{{Name: "coverage", Status: check.Failed, Error: "build failed"}}, "Go coverage: n/a", Unknown},
		{"no score", []check.Result{{Name: "coverage", Status: check.Info, Summary: "no statements"}}, "Go coverage: n/a", Unknown},
		{"not run", []check.Result{{Name: "errcheck", Score: score(1)}}, "Go coverage: n/a", Unknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rep := check.Report{Score: 90, Grade: check.GradeA, Checks: tt.checks}
			svg := Coverage(rep)
			s := string(svg)
			if !strings.Contains(s, `aria-label="`+tt.want+`"`) {
				t.Errorf("badge does not say %q:\n%s", tt.want, s)
			}
			if !strings.Contains(s, `fill="`+tt.color+`"`) {
				t.Errorf("badge is not %s:\n%s", tt.color, s)
			}
			if err := wellFormed(svg); err != nil {
				t.Errorf("not well-formed XML: %v", err)
			}
			if !bytes.Equal(svg, Coverage(rep)) {
				t.Error("output is not deterministic")
			}
		})
	}
}

func TestSVGEscapes(t *testing.T) {
	svg := SVG(`<a href="x">`, "&", `"red`)
	if err := wellFormed(svg); err != nil {
		t.Fatalf("not well-formed XML: %v\n%s", err, svg)
	}
	if bytes.Contains(svg, []byte("<a ")) {
		t.Errorf("markup in the label was not escaped:\n%s", svg)
	}
}

func TestColor(t *testing.T) {
	seen := make(map[string]bool)
	for _, g := range []check.Grade{check.GradeAPlus, check.GradeA, check.GradeB, check.GradeC, check.GradeD, check.GradeF} {
		seen[Color(g)] = true
	}
	seen[Unknown] = true
	if len(seen) != 7 {
		t.Errorf("grades share colors, or with n/a: %v", seen)
	}
}

func wellFormed(data []byte) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := d.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
