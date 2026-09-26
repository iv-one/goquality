package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repo creates a repository with a commit on main and one on a feature
// branch, which is checked out.
func repo(t *testing.T) (dir, mainCommit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir = t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("sub/a.txt", "one\n")
	git("add", ".")
	git("commit", "-q", "-m", "one")
	mainCommit = git("rev-parse", "HEAD")
	git("checkout", "-q", "-b", "feature")
	write("sub/a.txt", "two\n")
	git("commit", "-q", "-am", "two")
	return dir, mainCommit[:len(mainCommit)-1]
}

func TestRepo(t *testing.T) {
	dir, mainCommit := repo(t)
	ctx := context.Background()
	r, err := Open(ctx, filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}

	ref, err := r.DefaultRef(ctx)
	if err != nil || ref != "main" {
		t.Fatalf("DefaultRef = %q, %v; want main", ref, err)
	}
	base, err := r.MergeBase(ctx, ref)
	if err != nil || base != mainCommit {
		t.Fatalf("MergeBase = %q, %v; want %s", base, err, mainCommit)
	}
	if _, dirty, err := r.Head(ctx); err != nil || dirty {
		t.Errorf("Head: dirty = %v, err = %v; want clean", dirty, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, dirty, _ := r.Head(ctx); !dirty {
		t.Error("Head: untracked file does not make the tree dirty")
	}

	dst := t.TempDir()
	if err := r.Export(ctx, base, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "sub", "a.txt"))
	if err != nil || string(got) != "one\n" {
		t.Errorf("exported a.txt = %q, %v; want the main version", got, err)
	}

	if _, err := r.Commit(ctx, "nope"); err == nil {
		t.Error("Commit(nope): want an error")
	}
}
