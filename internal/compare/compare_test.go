package compare

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iv-one/goquality/internal/check"
)

func snapshot(results ...check.Result) Snapshot {
	return Snapshot{
		Schema:   Schema,
		Version:  "v1",
		Settings: Settings{Patterns: []string{"./..."}, CycloOver: 15},
		Report:   check.Report{Checks: results},
	}
}

func score(f float64) *float64 { return &f }

func finding(file string, line int, msg string) check.Finding {
	return check.Finding{File: file, Line: line, Message: msg}
}

func TestCompare(t *testing.T) {
	base := snapshot(
		check.Result{Name: "errcheck", Status: check.Warn, Score: score(0.5), Findings: []check.Finding{
			finding("a.go", 10, "unchecked error"),
			finding("a.go", 20, "unchecked error"),
		}},
		check.Result{Name: "complexity", Status: check.Warn, Score: score(0.9), Findings: []check.Finding{
			finding("b.go", 5, "cyclomatic complexity 17 of p.F is high (> 15)"),
		}},
		check.Result{Name: "gofmt", Status: check.Warn, Score: score(0.5), Findings: []check.Finding{
			finding("c.go", 1, "file is not gofmt-ed"),
		}},
		check.Result{Name: "coverage", Status: check.Pass, Score: score(0.7840)},
		check.Result{Name: "tests", Status: check.Pass, Score: score(1)},
		check.Result{Name: "gosec", Status: check.Skipped, Summary: "skipped"},
		check.Result{Name: "govulncheck", Status: check.Pass, Score: score(1)},
	)
	cur := snapshot(
		// Both shifted by an edit above them, plus a third one.
		check.Result{Name: "errcheck", Status: check.Warn, Score: score(0.4), Findings: []check.Finding{
			finding("a.go", 12, "unchecked error"),
			finding("a.go", 22, "unchecked error"),
			finding("a.go", 30, "unchecked error"),
		}},
		// Complexity went down but is still over the threshold.
		check.Result{Name: "complexity", Status: check.Warn, Score: score(0.9), Findings: []check.Finding{
			finding("b.go", 6, "cyclomatic complexity 16 of p.F is high (> 15)"),
		}},
		check.Result{Name: "gofmt", Status: check.Pass, Score: score(1)},
		check.Result{Name: "coverage", Status: check.Pass, Score: score(0.7805)},
		// Float noise below display precision.
		check.Result{Name: "tests", Status: check.Pass, Score: score(0.99999)},
		check.Result{Name: "gosec", Status: check.Pass, Score: score(1)},
		check.Result{Name: "license", Status: check.Pass, Score: score(1)},
		check.Result{Name: "govulncheck", Status: check.Skipped, Summary: "skipped (--no-security)"},
	)

	c, err := Compare(base, cur, "origin/main", "working tree")
	if err != nil {
		t.Fatal(err)
	}
	if c.Passed {
		t.Error("Passed = true, want false")
	}
	var got []string
	for _, d := range c.Checks {
		s := fmt.Sprintf("%s %s new=%d fixed=%d dropped=%v", d.Check, d.Status, len(d.New), len(d.Fixed), d.Dropped)
		if d.Reason != "" {
			s += " (" + d.Reason + ")"
		}
		got = append(got, s)
	}
	want := []string{
		"errcheck fail new=1 fixed=0 dropped=false",
		"complexity pass new=0 fixed=0 dropped=false",
		"gofmt pass new=0 fixed=1 dropped=false",
		"coverage fail new=0 fixed=0 dropped=true",
		"tests pass new=0 fixed=0 dropped=false",
		"gosec skipped new=0 fixed=0 dropped=false (baseline: skipped)",
		"license skipped new=0 fixed=0 dropped=false (not run on baseline)",
		"govulncheck skipped new=0 fixed=0 dropped=false (--no-security)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("checks:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if n := c.Checks[0].New; len(n) != 1 || n[0].Line != 30 {
		t.Errorf("new errcheck findings = %v, want the one on line 30", n)
	}
}

func TestDiffFindingsDuplicates(t *testing.T) {
	// Same message twice in a file; one of them removed, one added
	// elsewhere. Lines that still match pair up first.
	base := []check.Finding{finding("a.go", 3, "x"), finding("a.go", 9, "x")}
	cur := []check.Finding{finding("a.go", 9, "x"), finding("a.go", 15, "x"), finding("a.go", 20, "x")}
	added, fixed := diffFindings(base, cur)
	if len(added) != 1 || added[0].Line != 20 {
		t.Errorf("added = %v, want line 20", added)
	}
	if len(fixed) != 0 {
		t.Errorf("fixed = %v, want none", fixed)
	}
}

func TestComparePasses(t *testing.T) {
	s := snapshot(check.Result{Name: "errcheck", Status: check.Warn, Score: score(0.5), Findings: []check.Finding{finding("a.go", 1, "x")}})
	c, err := Compare(s, s, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Passed || len(c.Regressions()) != 0 {
		t.Errorf("comparing a snapshot with itself: %+v", c)
	}
}

func TestCompareSettings(t *testing.T) {
	base, cur := snapshot(), snapshot()
	cur.Settings.CycloOver = 20
	if _, err := Compare(base, cur, "a", "b"); err == nil || !strings.Contains(err.Error(), "cyclo-over") {
		t.Errorf("err = %v, want a --cyclo-over mismatch", err)
	}
	cur = snapshot()
	cur.Settings.Patterns = []string{"./cmd/..."}
	if _, err := Compare(base, cur, "a", "b"); err == nil {
		t.Error("comparing different packages: want an error")
	}
	cur = snapshot()
	cur.Version = "v2"
	c, err := Compare(base, cur, "a", "b")
	if err != nil || len(c.Notes) != 1 {
		t.Errorf("different versions: err = %v, notes = %v; want one note", err, c.Notes)
	}
}

func TestDiffFindingsLineHash(t *testing.T) {
	// A new identical finding inserted above an existing one: the line
	// hash, not the line number, tells which one is new.
	old := check.Finding{File: "a.go", Line: 12, Message: "x", LineHash: "remove"}
	base := []check.Finding{old}
	cur := []check.Finding{
		{File: "a.go", Line: 12, Message: "x", LineHash: "chdir"},
		{File: "a.go", Line: 13, Message: "x", LineHash: "remove"},
	}
	added, fixed := diffFindings(base, cur)
	if len(added) != 1 || added[0].LineHash != "chdir" || len(fixed) != 0 {
		t.Errorf("added = %v, fixed = %v; want the chdir finding added", added, fixed)
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\n\tx()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := check.Report{Checks: []check.Result{{Findings: []check.Finding{
		{File: "a.go", Line: 3},
		{File: "a.go", Line: 99},
		{File: "a.go"},
	}}}}
	Fingerprint(dir, &rep)
	fs := rep.Checks[0].Findings
	if fs[0].LineHash == "" || fs[1].LineHash != "" || fs[2].LineHash != "" {
		t.Errorf("hashes = %q, %q, %q; want only the first set", fs[0].LineHash, fs[1].LineHash, fs[2].LineHash)
	}
	// Indentation does not matter.
	other := check.Report{Checks: []check.Result{{Findings: []check.Finding{{File: "b.go", Line: 1}}}}}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("x()"), 0o644); err != nil {
		t.Fatal(err)
	}
	Fingerprint(dir, &other)
	if got := other.Checks[0].Findings[0].LineHash; got != fs[0].LineHash {
		t.Errorf("hash of %q = %s, want %s", "x()", got, fs[0].LineHash)
	}
}
