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
	if len(seen) != 6 {
		t.Errorf("grades share colors: %v", seen)
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
