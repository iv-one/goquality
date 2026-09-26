package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iv-one/goquality/internal/check"
	"github.com/iv-one/goquality/internal/compare"
	"github.com/iv-one/goquality/internal/git"
	"github.com/iv-one/goquality/internal/project"
	"github.com/iv-one/goquality/internal/report"
)

type logFunc func(format string, a ...any)

// runCollect writes a snapshot of the report.
func runCollect(ctx context.Context, o options, checks []check.Check, stdout, stderr io.Writer, logf logFunc) int {
	dir, patterns := target(o)
	status := newStatus(stderr, true)
	snap, err := collect(ctx, dir, patterns, checks, o, status)
	status.clear()
	if err != nil {
		logf("%v", err)
		return 2
	}
	if repo, err := git.Open(ctx, dir); err == nil {
		snap.Commit, snap.Dirty, _ = repo.Head(ctx)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		logf("%v", err)
		return 2
	}
	data = append(data, '\n')
	if o.output == "" {
		_, err = stdout.Write(data)
	} else {
		err = os.WriteFile(o.output, data, 0o644) //nolint:gosec // the user chooses where to write the snapshot; it is not secret
	}
	if err != nil {
		logf("%v", err)
		return 2
	}
	return 0
}

// runCompare compares two snapshot files.
func runCompare(o options, stdout io.Writer, logf logFunc) int {
	if len(o.args) != 2 {
		logf("compare needs two snapshot files: goquality compare baseline.json current.json")
		return 2
	}
	base, err := compare.Read(o.args[0])
	if err != nil {
		logf("%v", err)
		return 2
	}
	cur, err := compare.Read(o.args[1])
	if err != nil {
		logf("%v", err)
		return 2
	}
	c, err := compare.Compare(base, cur, o.args[0], o.args[1])
	if err != nil {
		logf("%v", err)
		return 2
	}
	return finish(stdout, c, o, logf)
}

// runCheck compares the working tree with a baseline revision or snapshot.
func runCheck(ctx context.Context, o options, checks []check.Check, stdout, stderr io.Writer, logf logFunc) int {
	dir, patterns := target(o)
	status := newStatus(stderr, !o.json && !o.agent)
	defer status.clear()

	repo, repoErr := git.Open(ctx, dir)
	var base compare.Snapshot
	var label string
	var err error
	if isSnapshotFile(o.baseline) {
		label = o.baseline
		base, err = compare.Read(o.baseline)
	} else if repoErr != nil {
		err = fmt.Errorf("check compares with a git revision: %w", repoErr)
	} else {
		label, base, err = collectBaseline(ctx, repo, o.baseline, dir, patterns, checks, o, status)
	}
	if err != nil {
		status.clear()
		logf("%v", err)
		return 2
	}

	cur, err := collect(ctx, dir, patterns, checks, o, status)
	if err != nil {
		status.clear()
		logf("%v", err)
		return 2
	}
	if repoErr == nil {
		cur.Commit, cur.Dirty, _ = repo.Head(ctx)
	}
	status.clear()

	c, err := compare.Compare(base, cur, label, "working tree")
	if err != nil {
		logf("%v", err)
		return 2
	}
	return finish(stdout, c, o, logf)
}

// collectBaseline exports the merge base of HEAD and ref (by default the
// remote's default branch) to a temporary directory and analyzes it with
// the same settings as the working tree.
func collectBaseline(ctx context.Context, repo *git.Repo, ref, dir string, patterns []string, checks []check.Check, o options, status *status) (string, compare.Snapshot, error) {
	var err error
	if ref == "" {
		if ref, err = repo.DefaultRef(ctx); err != nil {
			return "", compare.Snapshot{}, err
		}
	}
	if _, err := repo.Commit(ctx, ref); err != nil {
		return "", compare.Snapshot{}, fmt.Errorf("baseline %q: %w", ref, err)
	}
	commit, err := repo.MergeBase(ctx, ref)
	if err != nil {
		return "", compare.Snapshot{}, fmt.Errorf("no merge base with %s (in a shallow clone, fetch full history): %w", ref, err)
	}

	// The project may be a subdirectory of the repository.
	abs, err := filepath.Abs(dir)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return "", compare.Snapshot{}, err
	}
	rel, err := filepath.Rel(repo.Root, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", compare.Snapshot{}, fmt.Errorf("%s is not inside the repository %s", abs, repo.Root)
	}

	tmp, err := os.MkdirTemp("", "goquality-baseline-*")
	if err != nil {
		return "", compare.Snapshot{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// Resolve symlinks (/tmp on macOS) so paths match what go list reports.
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return "", compare.Snapshot{}, err
	}

	status.set(fmt.Sprintf("exporting %s@%.7s", ref, commit))
	if err := repo.Export(ctx, commit, tmp); err != nil {
		return "", compare.Snapshot{}, err
	}
	baseDir := filepath.Join(tmp, rel)
	if _, err := os.Stat(baseDir); err != nil {
		return "", compare.Snapshot{}, fmt.Errorf("%s does not exist at %s@%.7s", rel, ref, commit)
	}
	snap, err := collect(ctx, baseDir, patterns, checks, o, status)
	if err != nil {
		return "", compare.Snapshot{}, fmt.Errorf("baseline %s@%.7s: %w", ref, commit, err)
	}
	snap.Commit = commit
	return ref, snap, nil
}

// collect loads and analyzes a project into a snapshot. The caller fills in
// the commit.
func collect(ctx context.Context, dir string, patterns []string, checks []check.Check, o options, status *status) (compare.Snapshot, error) {
	status.set("loading packages")
	p, err := project.Load(ctx, dir, patterns)
	if err != nil {
		return compare.Snapshot{}, err
	}
	status.set("analyzing " + displayName(p))
	opts := runOptions(o)
	rep := check.Run(ctx, p, checks, opts)
	if ctx.Err() != nil {
		return compare.Snapshot{}, errors.New("interrupted")
	}
	compare.Fingerprint(p.Root, &rep)
	var names []string
	for _, c := range checks {
		names = append(names, c.Name())
	}
	cyclo := opts.CyclomaticThreshold
	if cyclo <= 0 {
		cyclo = 15
	}
	return compare.Snapshot{
		Schema:    compare.Schema,
		Version:   version(),
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Settings: compare.Settings{
			Patterns:  p.Patterns,
			Checks:    names,
			Cover:     o.cover,
			CycloOver: cyclo,
		},
		Report: rep,
	}, nil
}

// finish renders a comparison and returns the exit code: 1 on regressions.
func finish(w io.Writer, c compare.Comparison, o options, logf logFunc) int {
	var err error
	switch {
	case o.agent:
		_, err = io.WriteString(w, report.ComparisonAgent(c, report.AgentOptions{
			MaxFindings: o.maxFinds,
			Command:     rerunCommand(o),
		}))
	case o.json:
		var data []byte
		if data, err = report.ComparisonJSON(c); err == nil {
			_, err = w.Write(data)
		}
	default:
		_, err = io.WriteString(w, report.ComparisonText(c, report.TextOptions{
			Verbose: o.verbose,
			Color:   useColor(w, o),
		}))
	}
	if err != nil {
		logf("%v", err)
		return 2
	}
	if !c.Passed {
		return 1
	}
	return 0
}

// isSnapshotFile reports whether a --baseline names a snapshot file rather
// than a git ref.
func isSnapshotFile(baseline string) bool {
	if !strings.HasSuffix(baseline, ".json") {
		return false
	}
	fi, err := os.Stat(baseline)
	return err == nil && fi.Mode().IsRegular()
}
