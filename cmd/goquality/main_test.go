package main

import (
	"bytes"
	"encoding/json"
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
