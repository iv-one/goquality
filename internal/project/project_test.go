package project

import (
	"context"
	"testing"
)

func TestLoadStats(t *testing.T) {
	p, err := Load(context.Background(), "../testdata/sample", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.ModulePath != "example.com/sample" {
		t.Errorf("ModulePath = %q", p.ModulePath)
	}

	got := p.Stats()
	want := Stats{
		Module:         "example.com/sample",
		GoVersion:      "1.22",
		Packages:       3,
		TestedPackages: 1,
		GoFiles:        4,
		TestFiles:      1,
		GeneratedFiles: 1,
		Lines:          82,
		CodeLines:      38,
		TestCodeLines:  23,
		Functions:      5,
		Tests:          1, // Testhelper does not count
		Benchmarks:     1,
		FuzzTests:      1,
		Examples:       1,
	}
	if got != want {
		t.Errorf("Stats() =\n%+v\nwant\n%+v", got, want)
	}

	for _, f := range p.Files {
		if f.Rel == "gen/gen.go" && !f.Generated {
			t.Error("gen/gen.go not detected as generated")
		}
	}
}

func TestLoadOutsideModule(t *testing.T) {
	if _, err := Load(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("expected an error outside a module")
	}
}

func TestCountLines(t *testing.T) {
	src := "package x\n\n// comment\nvar s = `a\nb`\n/* block\n*/\n"
	total, code := countLines([]byte(src))
	if total != 7 || code != 3 {
		t.Errorf("countLines = %d, %d; want 7, 3", total, code)
	}
}
