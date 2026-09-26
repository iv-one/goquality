package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iv-one/goquality/internal/badge"
	"github.com/iv-one/goquality/internal/check"
	"github.com/iv-one/goquality/internal/compare"
)

// runBadges writes score.svg and grade.svg to o.output, from a snapshot
// (--from) or from a fresh analysis.
func runBadges(ctx context.Context, o options, checks []check.Check, stdout, stderr io.Writer, logf logFunc) int {
	var rep check.Report
	if o.from != "" {
		snap, err := compare.Read(o.from)
		if err != nil {
			logf("%v", err)
			return 2
		}
		rep = snap.Report
	} else {
		dir, patterns := target(o)
		status := newStatus(stderr, true)
		snap, err := collect(ctx, dir, patterns, checks, o, status)
		status.clear()
		if err != nil {
			logf("%v", err)
			return 2
		}
		rep = snap.Report
	}

	if err := os.MkdirAll(o.output, 0o750); err != nil {
		logf("%v", err)
		return 2
	}
	for _, b := range []struct {
		name string
		svg  []byte
	}{
		{"score.svg", badge.Score(rep)},
		{"grade.svg", badge.Grade(rep)},
	} {
		path := filepath.Join(o.output, b.name)
		if err := os.WriteFile(path, b.svg, 0o644); err != nil { //nolint:gosec // badges are public images; the user chooses the directory
			logf("%v", err)
			return 2
		}
		_, _ = fmt.Fprintln(stdout, path)
	}
	return 0
}
