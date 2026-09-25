// Package report renders a check.Report as text or JSON.
package report

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/iv-one/goquality/internal/check"
)

// JSON renders the report as indented JSON.
func JSON(rep check.Report) ([]byte, error) {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// TextOptions configure the text report.
type TextOptions struct {
	Verbose bool // list individual findings
	Color   bool // ANSI colors
}

const width = 42 // width of a "label ..... value" line, including indent

type printer struct {
	w    *strings.Builder
	opts TextOptions
}

func (p printer) color(code, s string) string {
	if !p.opts.Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p printer) line(indent int, label, value, valueColor string) {
	dots := width - indent - utf8.RuneCountInString(label) - utf8.RuneCountInString(value) - 2
	if dots < 2 {
		dots = 2
	}
	fmt.Fprintf(p.w, "%s%s %s %s\n", strings.Repeat(" ", indent), label,
		p.color("2", strings.Repeat(".", dots)), p.color(valueColor, value))
}

func (p printer) section(title string) {
	fmt.Fprintf(p.w, "\n%s\n", p.color("1", title))
}

// Text renders a human-readable report.
func Text(rep check.Report, opts TextOptions) string {
	w := new(strings.Builder)
	p := printer{w: w, opts: opts}
	s := rep.Stats

	title := p.color("1", "Go Quality")
	if s.Module != "" {
		title += "  " + p.color("2", s.Module)
	}
	fmt.Fprintln(w, title)
	fmt.Fprintln(w)
	p.line(0, "Grade", string(rep.Grade), gradeColor(rep.Grade))
	p.line(0, "Score", fmt.Sprintf("%.1f%%", rep.Score), gradeColor(rep.Grade))

	p.section("Project")
	if s.GoVersion != "" {
		p.line(2, "go version", s.GoVersion, "")
	}
	p.line(2, "packages", num(s.Packages), "")
	p.line(2, "go files", num(s.GoFiles), "")
	p.line(2, "test files", num(s.TestFiles), "")
	if s.GeneratedFiles > 0 {
		p.line(2, "generated files", num(s.GeneratedFiles), "")
	}
	p.line(2, "lines of code", num(s.CodeLines), "")
	p.line(2, "lines of test code", num(s.TestCodeLines), "")
	p.line(2, "functions", num(s.Functions), "")
	p.line(2, "dependencies", num(s.DirectDependencies+s.IndirectDependencies), "")
	p.line(2, "direct dependencies", num(s.DirectDependencies), "")

	for _, cat := range check.Categories {
		var results []check.Result
		for _, r := range rep.Checks {
			if r.Category == cat {
				results = append(results, r)
			}
		}
		if len(results) == 0 {
			continue
		}
		p.section(strings.ToUpper(string(cat[:1])) + string(cat[1:]))
		for _, r := range results {
			p.result(r)
		}
	}

	p.section("Overall")
	p.line(2, "issues", num(rep.Issues), "")
	p.line(2, "time", fmt.Sprintf("%.1fs", rep.Duration), "")
	if !opts.Verbose && rep.Issues > 0 {
		fmt.Fprintln(w, p.color("2", "\nRun with --verbose to list issues."))
	}
	return w.String()
}

func (p printer) result(r check.Result) {
	p.line(2, r.Label, r.Summary, statusColor(r))
	if r.Error != "" {
		fmt.Fprintf(p.w, "      %s\n", p.color("31", r.Error))
		return
	}
	for _, m := range r.Metrics {
		p.line(4, m.Label, value(m.Value), "")
	}
	if r.Note != "" {
		fmt.Fprintf(p.w, "      %s\n", p.color("2", r.Note))
	}
	if !p.opts.Verbose {
		return
	}
	for _, f := range r.Findings {
		loc := f.File
		if f.Line > 0 {
			loc += ":" + strconv.Itoa(f.Line)
			if f.Column > 0 {
				loc += ":" + strconv.Itoa(f.Column)
			}
		}
		msg := f.Message
		if f.Severity != "" {
			msg = "[" + f.Severity + "] " + msg
		}
		if f.Rule != "" && !strings.HasPrefix(f.Message, f.Rule) {
			msg += p.color("2", " ("+f.Rule+")")
		}
		if f.Fix != "" {
			msg += p.color("2", " → "+f.Fix)
		}
		if loc != "" {
			fmt.Fprintf(p.w, "      %s %s\n", p.color("36", loc), msg)
		} else {
			fmt.Fprintf(p.w, "      %s\n", msg)
		}
	}
}

func statusColor(r check.Result) string {
	switch r.Status {
	case check.Pass:
		return "32"
	case check.Warn:
		return "33"
	case check.Failed:
		return "31"
	case check.Skipped:
		return "2"
	}
	return ""
}

func gradeColor(g check.Grade) string {
	switch g {
	case check.GradeAPlus, check.GradeA:
		return "1;32"
	case check.GradeB, check.GradeC:
		return "1;33"
	}
	return "1;31"
}

func value(v any) string {
	switch v := v.(type) {
	case int:
		return num(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// num formats an integer with thousands separators.
func num(n int) string {
	s := strconv.Itoa(n)
	if n < 0 || len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
