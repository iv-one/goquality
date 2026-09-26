// Package git runs the few git commands goquality needs to compare a
// project against a baseline revision. git is only needed for that; the
// rest of goquality works without it.
package git

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultRefs are tried in order when no baseline is given: the remote's
// default branch, then the common names for it.
var DefaultRefs = []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"}

// Repo is a git work tree.
type Repo struct {
	Root string // top-level directory of the work tree
}

// Open finds the repository that contains dir.
func Open(ctx context.Context, dir string) (*Repo, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(out)
	if err != nil {
		return nil, err
	}
	return &Repo{Root: root}, nil
}

// Commit resolves ref to a full commit hash.
func (r *Repo) Commit(ctx context.Context, ref string) (string, error) {
	return run(ctx, r.Root, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
}

// MergeBase returns the best common ancestor of HEAD and ref, which is
// what a change on HEAD should be compared against.
func (r *Repo) MergeBase(ctx context.Context, ref string) (string, error) {
	return run(ctx, r.Root, "merge-base", "--end-of-options", "HEAD", ref)
}

// DefaultRef returns the first of DefaultRefs that exists.
func (r *Repo) DefaultRef(ctx context.Context) (string, error) {
	for _, ref := range DefaultRefs {
		if _, err := r.Commit(ctx, ref); err != nil {
			continue
		}
		// Name the branch origin/HEAD points to, e.g. origin/main.
		if name, err := run(ctx, r.Root, "rev-parse", "--abbrev-ref", ref); err == nil && name != "" {
			return name, nil
		}
		return ref, nil
	}
	return "", fmt.Errorf("none of %s exist; fetch the target branch or pass --baseline", strings.Join(DefaultRefs, ", "))
}

// Head returns the commit checked out and whether the work tree has
// uncommitted changes, including untracked files.
func (r *Repo) Head(ctx context.Context) (commit string, dirty bool, err error) {
	commit, err = r.Commit(ctx, "HEAD")
	if err != nil {
		return "", false, err
	}
	status, err := run(ctx, r.Root, "status", "--porcelain")
	if err != nil {
		return "", false, err
	}
	return commit, status != "", nil
}

// Export writes the tree of commit into dst, which must exist. It reads
// the tree with git archive and leaves the repository untouched.
func (r *Repo) Export(ctx context.Context, commit, dst string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", r.Root, "archive", "--format=tar", commit) //nolint:gosec // commit is a resolved hash
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	xerr := extract(out, dst)
	// Drain so git does not block on a full pipe if extraction failed.
	_, _ = io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive %s: %v: %s", commit, err, strings.TrimSpace(stderr.String()))
	}
	return xerr
}

// extract writes a tar stream into dst. Writes go through an os.Root, so
// no entry can escape dst.
func extract(r io.Reader, dst string) error {
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(h.Name)
		switch h.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(name, 0o750)
		case tar.TypeReg:
			err = writeFile(root, name, tr, h.FileInfo().Mode().Perm())
		case tar.TypeSymlink:
			if err = root.MkdirAll(filepath.Dir(name), 0o750); err == nil {
				err = root.Symlink(h.Linkname, name)
			}
		}
		if err != nil {
			return err
		}
	}
}

func writeFile(root *os.Root, name string, r io.Reader, perm os.FileMode) error {
	if err := root.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return errors.Join(err, f.Close())
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git subcommands; refs follow --end-of-options
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("git is not installed")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(string(out)), nil
}
