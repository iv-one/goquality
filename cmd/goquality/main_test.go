package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "../../internal/testdata/sample"

func TestRunJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{sample, "--json", "--no-security"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	var rep struct {
		Grade  string
		Checks []struct{ Name string }
	}
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Grade == "" {
		t.Error("missing grade")
	}
	for _, c := range rep.Checks {
		if c.Name == "gosec" || c.Name == "govulncheck" {
			t.Errorf("%s ran despite --no-security", c.Name)
		}
	}
}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{sample, "--no-security", "--min-score", "99.9"}, 1},
		{[]string{"--skip", "nope", sample}, 2},
		{[]string{"--only", "nope", sample}, 2},
		{[]string{"-C", t.TempDir()}, 2},
		{[]string{"--version"}, 0},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if got := run(tt.args, &stdout, &stderr); got != tt.want {
			t.Errorf("run(%q) = %d, want %d; stderr: %s", tt.args, got, tt.want, stderr.String())
		}
	}
}

func TestRunOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{sample, "--json", "--only", "errcheck,gofmt"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}
	var rep struct{ Checks []struct{ Name string } }
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range rep.Checks {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "errcheck,gofmt" {
		t.Errorf("checks = %v, want [errcheck gofmt]", names)
	}
}

func TestRunAgent(t *testing.T) {
	for _, tt := range []struct {
		name  string
		env   string
		args  []string
		agent bool
	}{
		{"flag", "", []string{"--agent"}, true},
		{"claude code", "1", nil, true},
		{"claude code, explicit off", "1", []string{"--agent=false"}, false},
		{"plain", "", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDECODE", tt.env)
			var stdout, stderr bytes.Buffer
			args := append([]string{sample, "--only", "errcheck"}, tt.args...)
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
			}
			got := strings.HasPrefix(stdout.String(), "goquality: grade")
			if got != tt.agent {
				t.Errorf("agent format = %v, want %v:\n%s", got, tt.agent, stdout.String())
			}
		})
	}
}

// sampleRepo copies the sample module into a new git repository with one
// commit on main and returns its directory.
func sampleRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(sample)); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "sample"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestRunCheck(t *testing.T) {
	dir := sampleRepo(t)
	args := []string{"check", "--only", "errcheck,gofmt", dir}

	var stdout, stderr bytes.Buffer
	if code := run(append(args, "--agent"), &stdout, &stderr); code != 0 {
		t.Fatalf("unchanged tree: exit code %d, want 0\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "goquality check: PASS, no regressions vs main@") {
		t.Errorf("unchanged tree:\n%s", stdout.String())
	}

	// Add an unchecked error above the existing one, which shifts it.
	path := filepath.Join(dir, "main.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src = bytes.Replace(src, []byte("func main() {\n"), []byte("func main() {\n\tos.Chdir(\"x\")\n"), 1)
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := run(append(args, "--json"), &stdout, &stderr); code != 1 {
		t.Fatalf("new finding: exit code %d, want 1\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	var c struct {
		Current struct{ Dirty bool }
		Checks  []struct {
			Check string
			New   []struct {
				File string
				Line int
			}
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Current.Dirty {
		t.Error("current side is not marked dirty")
	}
	var got []string
	for _, d := range c.Checks {
		for _, f := range d.New {
			got = append(got, fmt.Sprintf("%s %s:%d", d.Check, f.File, f.Line))
		}
	}
	if strings.Join(got, ",") != "errcheck main.go:12" {
		t.Errorf("new findings = %v, want [errcheck main.go:12]", got)
	}
}

func TestRunCollectCompare(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"collect", "--only", "errcheck", "-o", base, sample}, &stdout, &stderr); code != 0 {
		t.Fatalf("collect: exit code %d, stderr: %s", code, stderr.String())
	}
	if code := run([]string{"compare", "--json", base, base}, &stdout, &stderr); code != 0 {
		t.Fatalf("compare with itself: exit code %d, stderr: %s", code, stderr.String())
	}

	// A baseline without findings makes the sample's errcheck finding new.
	data, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	checks := snap["report"].(map[string]any)["checks"].([]any)
	delete(checks[0].(map[string]any), "findings")
	clean := filepath.Join(dir, "clean.json")
	data, _ = json.Marshal(snap)
	if err := os.WriteFile(clean, data, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := run([]string{"compare", "--agent=false", "--no-color", clean, base}, &stdout, &stderr); code != 1 {
		t.Fatalf("compare with regression: exit code %d, want 1; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "errcheck: 1 new finding\n      main.go:12:11 unchecked error") {
		t.Errorf("compare output:\n%s", stdout.String())
	}

	for _, args := range [][]string{
		{"compare", base},
		{"compare", base, filepath.Join(dir, "missing.json")},
		{"check", "--baseline", "nope", sample},
	} {
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) = %d, want 2", args, code)
		}
	}
}
