package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/securego/gosec/v2"
	"github.com/securego/gosec/v2/analyzers"
	"github.com/securego/gosec/v2/issue"
	"github.com/securego/gosec/v2/rules"
	"golang.org/x/tools/go/packages"
	"golang.org/x/vuln/scan"
)

// Govulncheck reports known vulnerabilities from the Go vulnerability
// database, using govulncheck's reachability analysis.
//
// Only vulnerabilities in code the project actually calls affect the score;
// vulnerable packages that are imported but not called, and vulnerable
// modules that are merely required, are reported as metrics.
func Govulncheck() Check {
	return checkFunc{name: "govulncheck", category: Security, weight: 0.10, run: runGovulncheck}
}

// govulncheck -json message stream (golang.org/x/vuln/internal/govulncheck).
type vulnMessage struct {
	OSV *struct {
		ID      string   `json:"id"`
		Summary string   `json:"summary"`
		Aliases []string `json:"aliases"`
	} `json:"osv"`
	Finding *struct {
		OSV          string `json:"osv"`
		FixedVersion string `json:"fixed_version"`
		Trace        []struct {
			Module   string `json:"module"`
			Version  string `json:"version"`
			Package  string `json:"package"`
			Function string `json:"function"`
			Receiver string `json:"receiver"`
			Position *struct {
				Filename string `json:"filename"`
				Line     int    `json:"line"`
				Column   int    `json:"column"`
			} `json:"position"`
		} `json:"trace"`
	} `json:"finding"`
}

// Reachability levels reported by govulncheck, from least to most precise.
const (
	vulnModule  = iota + 1 // vulnerable module version is required
	vulnPackage            // vulnerable package is imported
	vulnSymbol             // vulnerable symbol is called
)

type vuln struct {
	id, summary, module, version, fixed string
	level                               int
	finding                             Finding
}

func runGovulncheck(ctx context.Context, env *Env) Result {
	p := env.Project
	args := append([]string{"-C", p.Root, "-json"}, p.Patterns...)
	cmd := scan.Command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Start(); err != nil {
		return Result{Error: err.Error()}
	}
	runErr := cmd.Wait()
	vulns, err := parseVulns(&stdout)
	if runErr == nil {
		runErr = err
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return Result{Error: errorText(msg)}
	}
	return vulnResult(vulns)
}

// parseVulns reads a govulncheck -json stream and keeps, for each
// vulnerability, its most precise finding.
func parseVulns(r io.Reader) (map[string]*vuln, error) {
	summaries := make(map[string]string)
	vulns := make(map[string]*vuln)
	dec := json.NewDecoder(r)
	for {
		var m vulnMessage
		if err := dec.Decode(&m); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if m.OSV != nil {
			summaries[m.OSV.ID] = m.OSV.Summary
		}
		if m.Finding == nil || len(m.Finding.Trace) == 0 {
			continue
		}
		v := vulns[m.Finding.OSV]
		if v == nil {
			v = &vuln{id: m.Finding.OSV}
			vulns[m.Finding.OSV] = v
		}
		v.add(m)
	}
	for id, v := range vulns {
		v.summary = summaries[id]
	}
	return vulns, nil
}

func (v *vuln) add(m vulnMessage) {
	top := m.Finding.Trace[0]
	level := vulnModule
	switch {
	case top.Function != "":
		level = vulnSymbol
	case top.Package != "":
		level = vulnPackage
	}
	if level < v.level {
		return
	}
	v.level, v.module, v.version, v.fixed = level, top.Module, top.Version, m.Finding.FixedVersion
	if level != vulnSymbol || v.finding.File != "" {
		return
	}
	// The last frame is the project's entry point into the vulnerable code.
	if last := m.Finding.Trace[len(m.Finding.Trace)-1]; last.Position != nil {
		v.finding.File = filepath.ToSlash(last.Position.Filename)
		v.finding.Line = last.Position.Line
		v.finding.Column = last.Position.Column
	}
}

func vulnResult(vulns map[string]*vuln) Result {
	var ids []string
	for id := range vulns {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var findings []Finding
	counts := make(map[int]int)
	affected := make(map[string]bool)
	for _, id := range ids {
		v := vulns[id]
		counts[v.level]++
		if v.level != vulnSymbol {
			continue
		}
		affected[v.module] = true
		fix := "no fix available"
		if v.fixed != "" {
			fix = "fixed in " + v.fixed
		}
		f := v.finding
		f.Rule = id
		f.Message = fmt.Sprintf("%s: %s (%s@%s, %s)", id, v.summary, v.module, v.version, fix)
		findings = append(findings, f)
	}

	called := counts[vulnSymbol]
	return Result{
		Label:    "vulnerabilities",
		Summary:  strconv.Itoa(called),
		Score:    ptr(max(0, 1-0.25*float64(called))),
		Findings: findings,
		Metrics: []Metric{
			{Key: "affected_modules", Label: "affected modules", Value: len(affected)},
			{Key: "imported_not_called", Label: "imported, not called", Value: counts[vulnPackage]},
			{Key: "required_not_imported", Label: "required, not imported", Value: counts[vulnModule]},
		},
	}
}

// gosecExcluded are gosec rules left out because another check covers them:
// G104 (unhandled errors) duplicates errcheck.
var gosecExcluded = []string{"G104"}

// Gosec runs the gosec security analyzer on non-test code. Severities are
// gosec's own; only HIGH and MEDIUM findings affect the score.
func Gosec() Check {
	return checkFunc{name: "gosec", category: Security, weight: 0.10, run: runGosec}
}

func runGosec(_ context.Context, env *Env) Result {
	p := env.Project
	files := len(p.SourceFiles(false))

	// Reuse the already loaded packages rather than gosec's own loader, which
	// runs "go list" once per directory. Analyzer instances are not safe for
	// concurrent use, so each worker gets its own.
	jobs := make(chan *packages.Package)
	var (
		mu     sync.Mutex
		issues []*issue.Issue
		wg     sync.WaitGroup
	)
	for range min(runtime.GOMAXPROCS(0), len(p.Roots)) {
		wg.Go(func() {
			a := newGosec()
			for pkg := range jobs {
				if pkg.Types == nil || pkg.IllTyped {
					continue
				}
				a.CheckRules(pkg)
				a.CheckAnalyzers(pkg)
			}
			found, _, _ := a.Report()
			mu.Lock()
			issues = append(issues, found...)
			mu.Unlock()
		})
	}
	for _, pkg := range p.Roots {
		jobs <- pkg
	}
	close(jobs)
	wg.Wait()

	var findings, serious []Finding
	bySeverity := make(map[issue.Score]int)
	for _, is := range issues {
		if is.NoSec {
			continue
		}
		f := Finding{
			Rule:     is.RuleID,
			Severity: is.Severity.String(),
			Message:  fmt.Sprintf("%s (confidence: %s)", is.What, is.Confidence),
		}
		if pf := p.File(is.File); pf != nil {
			if pf.Generated || pf.Test {
				continue
			}
			f.File = pf.Rel
		} else {
			f.File = is.File
		}
		line, _, _ := strings.Cut(is.Line, "-")
		f.Line, _ = strconv.Atoi(line)
		f.Column, _ = strconv.Atoi(is.Col)
		if env.suppressed("gosec", f) {
			continue
		}
		bySeverity[is.Severity]++
		findings = append(findings, f)
		if is.Severity >= issue.Medium {
			serious = append(serious, f)
		}
	}
	return Result{
		Label:    "security findings",
		Summary:  strconv.Itoa(len(findings)),
		Findings: findings,
		Score:    fileScore(files, serious),
		Metrics: []Metric{
			{Key: "high", Label: "high", Value: bySeverity[issue.High]},
			{Key: "medium", Label: "medium", Value: bySeverity[issue.Medium]},
			{Key: "low", Label: "low", Value: bySeverity[issue.Low]},
		},
	}
}

func newGosec() *gosec.Analyzer {
	a := gosec.NewAnalyzer(gosec.NewConfig(), false, true, false, 1, log.New(io.Discard, "", 0))
	a.LoadRules(rules.Generate(false, rules.NewRuleFilter(true, gosecExcluded...)).RulesInfo())
	a.LoadAnalyzers(analyzers.Generate(false).AnalyzersInfo())
	return a
}
