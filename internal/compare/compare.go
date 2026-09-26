package compare

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/iv-one/goquality/internal/check"
)

// Side describes one of the compared snapshots.
type Side struct {
	Label      string      `json:"label"` // e.g. "origin/main" or a file name
	Commit     string      `json:"commit,omitempty"`
	Dirty      bool        `json:"dirty,omitempty"`
	Version    string      `json:"goquality_version"`
	Score      float64     `json:"score"` // 0..100
	Grade      check.Grade `json:"grade"`
	Suppressed int         `json:"suppressed"`
}

// Status is the outcome of comparing one check.
type Status string

// Check comparison outcomes.
const (
	Pass    Status = "pass"
	Fail    Status = "fail"
	Skipped Status = "skipped" // not run, or failed to run, on one side
)

// Delta compares one check across the two snapshots.
type Delta struct {
	Check   string          `json:"check"`
	Label   string          `json:"label"`
	Status  Status          `json:"status"`
	Base    *float64        `json:"base_score,omitempty"` // 0..1
	Current *float64        `json:"current_score,omitempty"`
	New     []check.Finding `json:"new,omitempty"`
	Fixed   []check.Finding `json:"fixed,omitempty"`
	Dropped bool            `json:"score_dropped,omitempty"` // the score fell with no findings on either side
	Reason  string          `json:"reason,omitempty"`        // why the check was skipped
}

// Comparison is the result of comparing a current snapshot to a baseline.
type Comparison struct {
	Module   string   `json:"module,omitempty"`
	Baseline Side     `json:"baseline"`
	Current  Side     `json:"current"`
	Passed   bool     `json:"passed"`
	Checks   []Delta  `json:"checks"`
	Notes    []string `json:"notes,omitempty"`
}

// Regressions returns the checks that failed.
func (c Comparison) Regressions() []Delta {
	var out []Delta
	for _, d := range c.Checks {
		if d.Status == Fail {
			out = append(out, d)
		}
	}
	return out
}

// Compare compares cur against base. A check fails when it reports a
// finding that base does not have, or, for a check without findings on
// either side (such as coverage), when its score fell. Checks that did not
// run on both sides are skipped.
func Compare(base, cur Snapshot, baseLabel, curLabel string) (Comparison, error) {
	if reason := comparable(base.Settings, cur.Settings); reason != "" {
		return Comparison{}, errors.New(reason)
	}
	c := Comparison{
		Module:   cur.Report.Stats.Module,
		Baseline: side(base, baseLabel),
		Current:  side(cur, curLabel),
		Passed:   true,
	}
	if base.Version != cur.Version {
		c.Notes = append(c.Notes, fmt.Sprintf("snapshots come from different goquality versions (%s vs %s); findings may differ for that reason", base.Version, cur.Version))
	}

	baseChecks := make(map[string]check.Result)
	for _, r := range base.Report.Checks {
		baseChecks[r.Name] = r
	}
	for _, r := range cur.Report.Checks {
		b, ok := baseChecks[r.Name]
		d := Delta{Check: r.Name, Label: r.Label, Base: b.Score, Current: r.Score}
		switch {
		case !ok:
			d.Status, d.Reason = Skipped, "not run on baseline"
		case !ran(r):
			d.Status, d.Reason = Skipped, notRun(r)
		case !ran(b):
			d.Status, d.Reason = Skipped, "baseline: "+notRun(b)
		default:
			d.New, d.Fixed = diffFindings(b.Findings, r.Findings)
			d.Dropped = len(b.Findings) == 0 && len(r.Findings) == 0 && lower(r.Score, b.Score)
			d.Status = Pass
			if len(d.New) > 0 || d.Dropped {
				d.Status = Fail
				c.Passed = false
			}
		}
		c.Checks = append(c.Checks, d)
	}
	return c, nil
}

func side(s Snapshot, label string) Side {
	return Side{
		Label:      label,
		Commit:     s.Commit,
		Dirty:      s.Dirty,
		Version:    s.Version,
		Score:      s.Report.Score,
		Grade:      s.Report.Grade,
		Suppressed: s.Report.Suppressed,
	}
}

func ran(r check.Result) bool {
	return r.Status != check.Skipped && r.Status != check.Failed
}

// notRun says why a check did not run, e.g. "--cover" for "skipped (--cover)".
func notRun(r check.Result) string {
	if r.Status == check.Failed {
		return "error: " + r.Error
	}
	reason := strings.TrimSpace(strings.TrimPrefix(r.Summary, "skipped"))
	reason = strings.TrimSuffix(strings.TrimPrefix(reason, "("), ")")
	if reason == "" {
		return "skipped"
	}
	return reason
}

// lower reports whether cur is below base at the report's display
// precision (0.1%), so floating-point noise does not count as a drop.
func lower(cur, base *float64) bool {
	if cur == nil || base == nil {
		return false
	}
	return math.Round(*cur*1000) < math.Round(*base*1000)
}

// digits matches numbers in finding messages. They are left out of a
// finding's identity so that, for example, a function whose complexity goes
// from 17 to 16 does not count as a new finding.
var digits = regexp.MustCompile(`[0-9]+`)

// key identifies a finding independently of its position, which shifts
// with unrelated edits to the same file.
func key(f check.Finding) string {
	return strings.Join([]string{f.File, f.Rule, digits.ReplaceAllString(f.Message, "N")}, "\x00")
}

// diffFindings matches findings by key. When a key occurs several times,
// findings on identical source lines pair up first and the rest pair up in
// order; those left over in cur are new and those left over in base are
// fixed.
func diffFindings(base, cur []check.Finding) (added, fixed []check.Finding) {
	byKey := make(map[string][]check.Finding)
	for _, f := range base {
		byKey[key(f)] = append(byKey[key(f)], f)
	}
	var unmatched []check.Finding
	for _, f := range cur {
		k := key(f)
		i := sameSource(byKey[k], f)
		if i < 0 {
			unmatched = append(unmatched, f)
			continue
		}
		byKey[k] = removeAt(byKey[k], i)
	}
	for _, f := range unmatched {
		k := key(f)
		if len(byKey[k]) > 0 {
			byKey[k] = byKey[k][1:]
			continue
		}
		added = append(added, f)
	}
	for _, fs := range byKey {
		fixed = append(fixed, fs...)
	}
	sort.SliceStable(fixed, func(i, j int) bool {
		if fixed[i].File != fixed[j].File {
			return fixed[i].File < fixed[j].File
		}
		return fixed[i].Line < fixed[j].Line
	})
	return added, fixed
}

// sameSource returns the index of the finding in fs on the same source
// line as f: by content when both have a line hash, else by line number.
func sameSource(fs []check.Finding, f check.Finding) int {
	for i, g := range fs {
		if f.LineHash != "" && g.LineHash != "" {
			if f.LineHash == g.LineHash {
				return i
			}
		} else if f.Line == g.Line {
			return i
		}
	}
	return -1
}

func removeAt(fs []check.Finding, i int) []check.Finding {
	return append(fs[:i:i], fs[i+1:]...)
}
