package check

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/iv-one/goquality/internal/project"
)

// TestCoverageAcrossPackages checks that code exercised only by another
// package's tests counts as covered.
func TestCoverageAcrossPackages(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":                "module example.com/cov\n\ngo 1.22\n",
		"api.go":                "package cov\n\nimport \"example.com/cov/internal/impl\"\n\n// Double doubles.\nfunc Double(n int) int { return impl.Double(n) }\n",
		"api_test.go":           "package cov\n\nimport \"testing\"\n\nfunc TestDouble(t *testing.T) {\n\tif Double(2) != 4 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n",
		"internal/impl/impl.go": "package impl\n\n// Double doubles.\nfunc Double(n int) int { return n * 2 }\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := project.Load(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := runCoverage(context.Background(), &Env{Project: p})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if r.Score == nil || *r.Score != 1 {
		t.Errorf("coverage = %s, want 100.0%%", r.Summary)
	}
}
