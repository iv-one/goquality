package check

import "sort"

// Step is a suggested improvement: bringing one check to 100%.
type Step struct {
	Check  string  `json:"check"`
	Label  string  `json:"label"`
	Gain   float64 `json:"gain"` // score points gained if the check fully passes
	Issues int     `json:"issues"`
	Files  int     `json:"files"`
	Hint   string  `json:"hint,omitempty"`
}

// hints tell a reader, human or agent, how to resolve a check's findings.
var hints = map[string]string{
	"build":       "Fix build and type errors first; other checks skip packages that do not compile.",
	"govet":       "Fix the reported problems; go vet findings are almost always real bugs.",
	"staticcheck": "Apply the suggested fixes; each rule is documented at https://staticcheck.dev/docs/checks/.",
	"errcheck":    "Handle the returned errors, or discard them explicitly with _ = where ignoring is intended.",
	"ineffassign": "Remove the dead assignment or use the assigned value.",
	"gofmt":       "Run gofmt -w on the listed files.",
	"complexity":  "Split the listed functions: extract branches and loops into well-named helpers.",
	"misspell":    "Correct the spelling.",
	"nolint":      "Fix the underlying issue, or write //nolint:<linter> // <reason>.",
	"license":     "Add a LICENSE file at the module root.",
	"tests":       "Add _test.go files for the listed packages.",
	"coverage":    "Add tests for uncovered code; go test -coverprofile=c.out ./... && go tool cover -func=c.out shows the gaps.",
	"govulncheck": "Upgrade the affected modules, or the Go toolchain for stdlib, to the fixed versions.",
	"gosec":       "Fix HIGH and MEDIUM findings first; they are the ones that affect the score.",
}

// nextSteps ranks the checks that are not at 100% by how much the overall
// score would rise if they were.
func nextSteps(results []Result, totalWeight float64) []Step {
	if totalWeight == 0 {
		return nil
	}
	var steps []Step
	for _, r := range results {
		if r.Score == nil || *r.Score >= 1 || r.Weight == 0 {
			continue
		}
		files := make(map[string]bool)
		for _, f := range r.Findings {
			files[f.File] = true
		}
		steps = append(steps, Step{
			Check:  r.Name,
			Label:  r.Label,
			Gain:   r.Weight * (1 - *r.Score) / totalWeight * 100,
			Issues: len(r.Findings),
			Files:  len(files),
			Hint:   hints[r.Name],
		})
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Gain > steps[j].Gain })
	return steps
}

// gradeFloors are the scores a report must exceed for each grade.
var gradeFloors = []struct {
	grade Grade
	floor float64
}{
	{GradeAPlus, 90}, {GradeA, 80}, {GradeB, 70}, {GradeC, 60}, {GradeD, 50}, {GradeE, 40},
}

// NextGrade returns the grade above g and the score it requires (exclusive),
// or ok=false when g is already the top grade.
func NextGrade(g Grade) (next Grade, floor float64, ok bool) {
	for i, gf := range gradeFloors {
		if gf.grade == g {
			if i == 0 {
				return "", 0, false
			}
			return gradeFloors[i-1].grade, gradeFloors[i-1].floor, true
		}
	}
	// F: the next grade is E.
	last := gradeFloors[len(gradeFloors)-1]
	return last.grade, last.floor, true
}

// StepsToGrade returns how many of the leading steps are needed to exceed
// floor, or 0 if fixing every step is not enough.
func (r Report) StepsToGrade(floor float64) int {
	score := r.Score
	for i, s := range r.NextSteps {
		score += s.Gain
		if score > floor {
			return i + 1
		}
	}
	return 0
}
