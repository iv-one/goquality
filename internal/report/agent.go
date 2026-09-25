package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/iv-one/goquality/internal/check"
)

// AgentOptions configure the agent report.
type AgentOptions struct {
	// MaxFindings caps the findings listed; 0 means no limit.
	MaxFindings int
	// Command is how to re-run goquality on the same target, e.g.
	// "goquality --agent ./pkg/...". Check names are appended with --only.
	Command string
}

// Agent renders a compact plain-text report for coding agents: a one-line
// verdict, what to fix next and how to re-check, then findings grouped by
// file. It avoids decoration that costs tokens without adding information.
func Agent(rep check.Report, opts AgentOptions) string {
	w := new(strings.Builder)
	s := rep.Stats
	name := s.Module
	if name == "" {
		name = "project"
	}
	fmt.Fprintf(w, "goquality: grade %s (%.1f%%), %s", rep.Grade, rep.Score, plural(rep.Issues, "issue"))
	if rep.Suppressed > 0 {
		fmt.Fprintf(w, ", %d suppressed", rep.Suppressed)
	}
	fmt.Fprintf(w, " | %s: %s, %s lines of code, %s\n",
		name, plural(s.Packages, "package"), num(s.CodeLines), plural(s.Tests, "test"))

	agentChecks(w, rep)
	agentSteps(w, rep, opts)
	agentFindings(w, rep, opts)

	w.WriteString("\nrules:\n")
	w.WriteString("- Fix the code rather than suppressing findings. Suppressions are counted, and a //nolint must name its linters and give a reason.\n")
	fmt.Fprintf(w, "- After changes, re-run %s to confirm the score went up.\n", opts.Command)
	return w.String()
}

// agentChecks lists check outcomes by status. Failing checks are detailed
// in the next steps, so they are only named here.
func agentChecks(w *strings.Builder, rep check.Report) {
	var pass, fail, skip []string
	var errs []string
	for _, r := range rep.Checks {
		switch r.Status {
		case check.Pass, check.Info:
			pass = append(pass, r.Name)
		case check.Skipped:
			reason := strings.TrimSpace(strings.TrimPrefix(r.Summary, "skipped"))
			skip = append(skip, strings.TrimSpace(r.Name+" "+reason))
		case check.Failed:
			errs = append(errs, fmt.Sprintf("%s (%s)", r.Name, r.Error))
		default:
			fail = append(fail, r.Name)
		}
	}
	w.WriteString("\nchecks:\n")
	for _, group := range []struct {
		label string
		names []string
	}{{"fail", fail}, {"error", errs}, {"skip", skip}, {"pass", pass}} {
		if len(group.names) > 0 {
			fmt.Fprintf(w, "  %s: %s\n", group.label, strings.Join(group.names, ", "))
		}
	}
}

func agentSteps(w *strings.Builder, rep check.Report, opts AgentOptions) {
	if len(rep.NextSteps) == 0 {
		return
	}
	title := "next steps, by score gain"
	if target := gradeTarget(rep); target != "" {
		title += " (" + target + ")"
	}
	fmt.Fprintf(w, "\n%s:\n", title)
	for i, s := range rep.NextSteps {
		fmt.Fprintf(w, "  %d. %s +%.1f%%", i+1, s.Check, s.Gain)
		if s.Issues > 0 {
			fmt.Fprintf(w, " (%s)", issuesIn(s))
		}
		if n := suppressedFor(rep, s.Check); n > 0 {
			fmt.Fprintf(w, " [%d suppressed]", n)
		}
		if s.Hint != "" {
			fmt.Fprintf(w, ": %s", s.Hint)
		}
		w.WriteString("\n")
	}
	fmt.Fprintf(w, "  Re-check a single check: %s --only <check>\n", opts.Command)
}

func suppressedFor(rep check.Report, name string) int {
	for _, r := range rep.Checks {
		if r.Name == name {
			return r.Suppressed
		}
	}
	return 0
}

type agentFinding struct {
	check string
	check.Finding
}

func agentFindings(w *strings.Builder, rep check.Report, opts AgentOptions) {
	// Take findings from the highest-gain checks first, so a capped list
	// still shows what matters most.
	order := make(map[string]int)
	for i, s := range rep.NextSteps {
		order[s.Check] = i
	}
	checks := append([]check.Result(nil), rep.Checks...)
	sort.SliceStable(checks, func(i, j int) bool {
		oi, iok := order[checks[i].Name]
		oj, jok := order[checks[j].Name]
		if iok != jok {
			return iok
		}
		return oi < oj
	})

	var all []agentFinding
	for _, r := range checks {
		for _, f := range r.Findings {
			all = append(all, agentFinding{r.Name, f})
		}
	}
	if len(all) == 0 {
		return
	}
	shown := all
	if opts.MaxFindings > 0 && len(shown) > opts.MaxFindings {
		shown = shown[:opts.MaxFindings]
	}
	sort.SliceStable(shown, func(i, j int) bool {
		a, b := shown[i], shown[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})

	if len(shown) < len(all) {
		fmt.Fprintf(w, "\nfindings (%d of %d; narrow with --only <check> or a package pattern):\n", len(shown), len(all))
	} else {
		fmt.Fprintf(w, "\nfindings (%d):\n", len(all))
	}
	file := "\x00"
	for _, f := range shown {
		if f.File != file {
			file = f.File
			if file == "" {
				w.WriteString("(project)\n")
			} else {
				w.WriteString(file + "\n")
			}
		}
		fmt.Fprintf(w, "  %s\n", agentLine(f))
	}
}

func agentLine(f agentFinding) string {
	var b strings.Builder
	if f.Line > 0 {
		fmt.Fprintf(&b, "%d", f.Line)
		if f.Column > 0 {
			fmt.Fprintf(&b, ":%d", f.Column)
		}
		b.WriteString(" ")
	}
	b.WriteString(f.check)
	if f.Rule != "" && !strings.HasPrefix(f.Message, f.Rule) {
		b.WriteString("/" + f.Rule)
	}
	if f.Severity != "" {
		b.WriteString(" " + f.Severity)
	}
	b.WriteString(": " + f.Message)
	if f.Fix != "" {
		b.WriteString(" [fix: " + f.Fix + "]")
	}
	return b.String()
}
