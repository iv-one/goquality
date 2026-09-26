package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iv-one/goquality/internal/check"
	"github.com/iv-one/goquality/internal/compare"
)

// ComparisonJSON renders a comparison as indented JSON.
func ComparisonJSON(c compare.Comparison) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ComparisonText renders a human-readable comparison: one line per check,
// then the regressions in detail. Verbose also lists fixed findings.
func ComparisonText(c compare.Comparison, opts TextOptions) string {
	w := new(strings.Builder)
	p := printer{w: w, opts: opts}

	title := p.color("1", "Go Quality")
	if c.Module != "" {
		title += "  " + p.color("2", c.Module)
	}
	fmt.Fprintln(w, title)
	fmt.Fprintln(w)
	p.line(0, "Baseline", describe(c.Baseline), "")
	p.line(0, "Current", describe(c.Current), "")
	p.line(0, "Score", fmt.Sprintf("%.1f%% → %.1f%%", c.Baseline.Score, c.Current.Score), "")
	if c.Baseline.Suppressed != c.Current.Suppressed {
		p.line(0, "Suppressed", fmt.Sprintf("%d → %d", c.Baseline.Suppressed, c.Current.Suppressed), "")
	}

	p.section("Checks")
	for _, d := range c.Checks {
		value, color := deltaSummary(d)
		p.line(2, d.Label, value, color)
	}

	if regs := c.Regressions(); len(regs) > 0 {
		p.section("Regressions")
		for _, d := range regs {
			fmt.Fprintf(w, "  %s: %s\n", d.Label, regression(d))
			for _, f := range d.New {
				p.finding(f)
			}
			if h := check.Hint(d.Check); h != "" {
				fmt.Fprintf(w, "      %s\n", p.color("2", h))
			}
		}
	}
	if opts.Verbose {
		var fixed []compare.Delta
		for _, d := range c.Checks {
			if len(d.Fixed) > 0 {
				fixed = append(fixed, d)
			}
		}
		if len(fixed) > 0 {
			p.section("Fixed")
			for _, d := range fixed {
				fmt.Fprintf(w, "  %s:\n", d.Label)
				for _, f := range d.Fixed {
					p.finding(f)
				}
			}
		}
	}
	for _, n := range c.Notes {
		fmt.Fprintf(w, "\n%s\n", p.color("2", "Note: "+n))
	}

	fmt.Fprintln(w)
	if c.Passed {
		fmt.Fprintln(w, p.color("1;32", "PASSED")+": no regressions against "+c.Baseline.Label)
	} else {
		n := len(c.Regressions())
		fmt.Fprintf(w, "%s: %s regressed against %s\n", p.color("1;31", "FAILED"), plural(n, "check"), c.Baseline.Label)
	}
	return w.String()
}

// ComparisonAgent renders a comparison compactly for coding agents: a
// verdict, then each regression with its new findings and a fix hint.
func ComparisonAgent(c compare.Comparison, opts AgentOptions) string {
	w := new(strings.Builder)
	verdict := "PASS, no regressions"
	if !c.Passed {
		verdict = fmt.Sprintf("FAIL, %s regressed", plural(len(c.Regressions()), "check"))
	}
	fmt.Fprintf(w, "goquality check: %s vs %s | score %.1f%% -> %.1f%%", verdict, describe(c.Baseline), c.Baseline.Score, c.Current.Score)
	if c.Baseline.Suppressed != c.Current.Suppressed {
		fmt.Fprintf(w, ", suppressed %d -> %d", c.Baseline.Suppressed, c.Current.Suppressed)
	}
	if c.Module != "" {
		fmt.Fprintf(w, " | %s", c.Module)
	}
	w.WriteString("\n")

	agentRegressions(w, c, opts)
	agentOutcomes(w, c)

	if !c.Passed {
		w.WriteString("\nrules:\n")
		w.WriteString("- Fix the new findings rather than suppressing them. Suppressions are counted, and a //nolint must name its linters and give a reason.\n")
		fmt.Fprintf(w, "- After changes, re-run %s to confirm there are no regressions.\n", opts.Command)
	}
	return w.String()
}

// agentRegressions lists each failed check with its hint and new
// findings, capped at opts.MaxFindings in total.
func agentRegressions(w *strings.Builder, c compare.Comparison, opts AgentOptions) {
	regs := c.Regressions()
	if len(regs) == 0 {
		return
	}
	w.WriteString("\nregressions:\n")
	shown := 0
	for _, d := range regs {
		fmt.Fprintf(w, "  %s: %s", d.Check, regression(d))
		if h := check.Hint(d.Check); h != "" {
			fmt.Fprintf(w, ": %s", h)
		}
		w.WriteString("\n")
		for _, f := range d.New {
			if opts.MaxFindings > 0 && shown == opts.MaxFindings {
				break
			}
			shown++
			fmt.Fprintf(w, "    %s\n", agentLocatedLine(d.Check, f))
		}
	}
	if total := newCount(c); opts.MaxFindings > 0 && total > shown {
		fmt.Fprintf(w, "  (%d of %d new findings shown; narrow with --only <check>)\n", shown, total)
	}
}

// agentLocatedLine is agentLine prefixed with the file, since findings of
// a comparison are grouped by check rather than by file.
func agentLocatedLine(name string, f check.Finding) string {
	line := agentLine(agentFinding{name, f})
	switch {
	case f.File != "" && f.Line > 0:
		return f.File + ":" + line
	case f.File != "":
		return f.File + " " + line
	}
	return line
}

// agentOutcomes lists fixed findings, checks that were not compared, and
// notes.
func agentOutcomes(w *strings.Builder, c compare.Comparison) {
	var fixed, skipped []string
	for _, d := range c.Checks {
		if len(d.Fixed) > 0 {
			fixed = append(fixed, fmt.Sprintf("%s %d", d.Check, len(d.Fixed)))
		}
		if d.Status == compare.Skipped {
			skipped = append(skipped, fmt.Sprintf("%s (%s)", d.Check, d.Reason))
		}
	}
	if len(fixed) > 0 {
		fmt.Fprintf(w, "\nfixed: %s\n", strings.Join(fixed, ", "))
	}
	if len(skipped) > 0 {
		fmt.Fprintf(w, "not compared: %s\n", strings.Join(skipped, ", "))
	}
	for _, n := range c.Notes {
		fmt.Fprintf(w, "note: %s\n", n)
	}
}

func newCount(c compare.Comparison) int {
	n := 0
	for _, d := range c.Checks {
		n += len(d.New)
	}
	return n
}

// describe names a compared side, e.g. "origin/main@6dbe334" or
// "working tree@6dbe334+changes".
func describe(s compare.Side) string {
	out := s.Label
	if s.Commit != "" && !strings.HasPrefix(s.Commit, s.Label) {
		out += "@" + short(s.Commit)
	}
	if s.Dirty {
		out += "+changes"
	}
	return out
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// deltaSummary is a check's value column: its status and what changed.
func deltaSummary(d compare.Delta) (string, string) {
	switch d.Status {
	case compare.Skipped:
		return "not compared (" + d.Reason + ")", "2"
	case compare.Fail:
		return "FAIL (" + regression(d) + ")", "31"
	}
	if len(d.Fixed) > 0 {
		return fmt.Sprintf("PASS (%d fixed)", len(d.Fixed)), "32"
	}
	return "PASS", "32"
}

func regression(d compare.Delta) string {
	var parts []string
	if len(d.New) > 0 {
		parts = append(parts, plural(len(d.New), "new finding"))
	}
	if d.Dropped {
		parts = append(parts, fmt.Sprintf("%s → %s", percent(d.Base), percent(d.Current)))
	}
	return strings.Join(parts, ", ")
}

func percent(score *float64) string {
	if score == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", *score*100)
}
