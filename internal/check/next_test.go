package check

import (
	"math"
	"testing"
)

func TestNextSteps(t *testing.T) {
	results := []Result{
		{Name: "gofmt", Label: "gofmt", Weight: 0.2, Score: ptr(0.5), Findings: []Finding{{File: "a.go"}, {File: "b.go"}}},
		{Name: "errcheck", Label: "errcheck", Weight: 0.4, Score: ptr(0.75), Findings: []Finding{{File: "a.go"}, {File: "a.go"}}},
		{Name: "govet", Label: "go vet", Weight: 0.4, Score: ptr(1)},
		{Name: "coverage", Label: "coverage", Weight: 0.1, Status: Skipped},
	}
	steps := nextSteps(results, 1.0, 80)
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2: %+v", len(steps), steps)
	}
	// Both would add 10 points; the stable sort keeps report order.
	if steps[0].Check != "gofmt" || steps[1].Check != "errcheck" {
		t.Errorf("order = %s, %s", steps[0].Check, steps[1].Check)
	}
	if steps[0].Gain != 10 || steps[0].Files != 2 || steps[1].Files != 1 || steps[1].Issues != 2 {
		t.Errorf("steps = %+v", steps)
	}
	if steps[0].Hint == "" {
		t.Error("missing hint")
	}

	rep := Report{Score: 75, NextSteps: steps}
	if n := rep.StepsToGrade(80); n != 1 {
		t.Errorf("StepsToGrade(80) = %d, want 1", n)
	}
	if n := rep.StepsToGrade(90); n != 2 {
		t.Errorf("StepsToGrade(90) = %d, want 2", n)
	}
	if n := rep.StepsToGrade(99); n != 0 {
		t.Errorf("StepsToGrade(99) = %d, want 0", n)
	}
}

func TestNextStepsBlockers(t *testing.T) {
	results := []Result{
		{Name: "gofmt", Weight: 0.5, Score: ptr(0.9)},
		{Name: "gosec", Weight: 0.5, Score: ptr(0.98), Findings: []Finding{{File: "a.go", Blocker: true}}},
	}
	// Uncapped 97% is shown as 80% while the blocker remains.
	steps := nextSteps(results, 1.0, 97)
	if len(steps) != 2 || steps[0].Check != "gosec" || steps[0].Blockers != 1 {
		t.Fatalf("steps = %+v, want gosec first", steps)
	}
	// Fixing gosec lifts the cap: 80% to 98%. gofmt then adds its 5 points.
	if !approx(steps[0].Gain, 18) || !approx(steps[1].Gain, 5) {
		t.Errorf("gains = %v, %v; want 18, 5", steps[0].Gain, steps[1].Gain)
	}

	// Two checks with blockers: the first fix alone does not lift the cap.
	results[0].Findings = []Finding{{File: "b.go", Blocker: true}}
	steps = nextSteps(results, 1.0, 97)
	if !approx(steps[0].Gain, 0) || !approx(steps[1].Gain, 23) {
		t.Errorf("gains = %v, %v; want 0, 23", steps[0].Gain, steps[1].Gain)
	}
}

func TestCapScore(t *testing.T) {
	if got := capScore(97, true); got != BlockerCap {
		t.Errorf("capScore(97, true) = %v", got)
	}
	if got := capScore(60, true); got != 60 {
		t.Errorf("capScore(60, true) = %v", got)
	}
	if got := capScore(97, false); got != 97 {
		t.Errorf("capScore(97, false) = %v", got)
	}
	if g := GradeFromPercentage(BlockerCap); g != GradeB {
		t.Errorf("capped grade = %s, want B", g)
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestNextGrade(t *testing.T) {
	tests := []struct {
		g     Grade
		next  Grade
		floor float64
		ok    bool
	}{
		{GradeAPlus, "", 0, false},
		{GradeA, GradeAPlus, 90, true},
		{GradeB, GradeA, 80, true},
		{GradeF, GradeE, 40, true},
	}
	for _, tt := range tests {
		next, floor, ok := NextGrade(tt.g)
		if next != tt.next || floor != tt.floor || ok != tt.ok {
			t.Errorf("NextGrade(%s) = %s, %v, %v", tt.g, next, floor, ok)
		}
	}
}
