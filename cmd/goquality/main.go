// Command goquality reports the quality, maintainability, test and security
// health of a local Go project.
//
// Usage:
//
//	goquality [flags] [dir | packages]
//	goquality collect [flags] [dir | packages]
//	goquality compare [flags] baseline.json current.json
//	goquality check [--baseline ref|file.json] [flags] [dir | packages]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"

	"github.com/iv-one/goquality/internal/check"
	"github.com/iv-one/goquality/internal/git"
	"github.com/iv-one/goquality/internal/project"
	"github.com/iv-one/goquality/internal/report"
)

var usages = map[string]string{
	"report": `goquality reports the health of a Go project.

Usage:
  goquality [flags] [dir | packages]
  goquality collect | compare | check ... (run "goquality <command> -h")

With no arguments, goquality analyzes ./... in the current directory.
A single directory argument analyzes that directory recursively; otherwise
arguments are package patterns, as for "go build".

Commands:
  collect   write a snapshot of the report, to compare later
  compare   compare two snapshots and fail on regressions
  check     compare the working tree with a baseline revision

Flags:
`,
	"collect": `Collect writes a snapshot of the report as JSON, with the commit and
settings it was produced with.

Usage:
  goquality collect [flags] [dir | packages]

Flags:
`,
	"compare": `Compare compares two snapshots written by "goquality collect".

Usage:
  goquality compare [flags] baseline.json current.json

It fails when a check reports a finding that the baseline does not have,
or when the score of a check without findings (such as coverage) falls.

Exit codes: 0 no regressions, 1 regressions, 2 usage or load error.

Flags:
`,
	"check": `Check compares the working tree, including uncommitted changes, with a
baseline. By default that is the merge base of HEAD and the first of
  ` + strings.Join(git.DefaultRefs, ", ") + `
that exists. The baseline revision is exported to a temporary directory and
analyzed with the same settings.

Usage:
  goquality check [--baseline ref | file.json] [flags] [dir | packages]

It fails when a check reports a finding that the baseline does not have,
or when the score of a check without findings (such as coverage) falls.

Exit codes: 0 no regressions, 1 regressions, 2 usage or load error.

Flags:
`,
}

type options struct {
	cmd        string // "report", "collect", "compare" or "check"
	dir        string
	verbose    bool
	json       bool
	agent      bool
	maxFinds   int
	cover      bool
	noSecurity bool
	noColor    bool
	skip       string
	only       string
	minScore   float64
	cyclo      int
	version    bool
	output     string   // collect: file to write the snapshot to
	baseline   string   // check: git ref or snapshot file
	args       []string // positional arguments
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes goquality and returns the process exit code: 0 on success,
// 1 when the score is below --min-score or a comparison finds regressions,
// 2 on usage or load errors.
func run(args []string, stdout, stderr io.Writer) int {
	logf := func(format string, a ...any) {
		_, _ = fmt.Fprintf(stderr, "goquality: "+format+"\n", a...)
	}

	cmd, args := command(args)
	o, err := parseFlags(cmd, args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	o.agent = useAgentFormat(o, args, stdout)
	if o.version {
		_, err := fmt.Fprintln(stdout, "goquality", version())
		return exitCode(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if cmd == "compare" {
		return runCompare(o, stdout, logf)
	}
	checks, err := selectChecks(o)
	if err != nil {
		logf("%v", err)
		return 2
	}
	switch cmd {
	case "collect":
		return runCollect(ctx, o, checks, stdout, stderr, logf)
	case "check":
		return runCheck(ctx, o, checks, stdout, stderr, logf)
	}

	dir, patterns := target(o)
	status := newStatus(stderr, !o.json && !o.agent)
	status.set("loading packages")
	p, err := project.Load(ctx, dir, patterns)
	if err != nil {
		status.clear()
		logf("%v", err)
		return 2
	}

	status.set("analyzing " + displayName(p))
	rep := check.Run(ctx, p, checks, runOptions(o))
	status.clear()
	if ctx.Err() != nil {
		logf("interrupted")
		return 2
	}

	if err := render(stdout, rep, o); err != nil {
		logf("%v", err)
		return 2
	}
	if o.minScore > 0 && rep.Score < o.minScore {
		if !o.json {
			logf("score %.1f%% is below the minimum of %.1f%%", rep.Score, o.minScore)
		}
		return 1
	}
	return 0
}

// command splits off the subcommand, if any; plain goquality is "report".
func command(args []string) (string, []string) {
	if len(args) > 0 {
		switch args[0] {
		case "collect", "compare", "check":
			return args[0], args[1:]
		}
	}
	return "report", args
}

func parseFlags(cmd string, args []string, stderr io.Writer) (options, error) {
	o := options{cmd: cmd}
	name := "goquality"
	if cmd != "report" {
		name += " " + cmd
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	if cmd != "compare" {
		fs.StringVar(&o.dir, "C", ".", "change to `dir` before running")
		fs.BoolVar(&o.cover, "cover", false, "run tests to measure coverage (executes project code)")
		fs.BoolVar(&o.noSecurity, "no-security", false, "skip security checks (govulncheck, gosec)")
		fs.StringVar(&o.skip, "skip", "", "comma-separated `checks` to skip, e.g. misspell,gosec")
		fs.StringVar(&o.only, "only", "", "comma-separated `checks` to run, e.g. errcheck,govet (for quick re-checks)")
		fs.IntVar(&o.cyclo, "cyclo-over", 15, "report functions with cyclomatic complexity above `n`")
	}
	if cmd != "collect" {
		fs.BoolVar(&o.verbose, "verbose", false, "list individual findings")
		fs.BoolVar(&o.verbose, "v", false, "shorthand for --verbose")
		fs.BoolVar(&o.json, "json", false, "print the report as JSON")
		fs.BoolVar(&o.agent, "agent", false, "compact report for coding agents (default when run by Claude Code without a terminal)")
		fs.IntVar(&o.maxFinds, "max-findings", 50, "findings listed by --agent (0 for all)")
		fs.BoolVar(&o.noColor, "no-color", false, "disable colored output")
	}
	switch cmd {
	case "report":
		fs.Float64Var(&o.minScore, "min-score", 0, "exit with status 1 if the score is below `percent`")
		fs.BoolVar(&o.version, "version", false, "print version and exit")
	case "collect":
		fs.StringVar(&o.output, "o", "", "write the snapshot to `file` instead of standard output")
	case "check":
		fs.StringVar(&o.baseline, "baseline", "", "git `ref` or snapshot file to compare with (default: the merge base with origin's default branch)")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, usages[cmd])
		fs.PrintDefaults()
		if cmd != "compare" {
			_, _ = fmt.Fprintf(stderr, "\nChecks: %s\n", strings.Join(checkNames(), ", "))
		}
	}

	// Allow flags after positional arguments: goquality ./... --json
	for {
		if err := fs.Parse(args); err != nil {
			return o, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return o, nil
		}
		o.args = append(o.args, args[0])
		args = args[1:]
	}
}

// useAgentFormat reports whether to print the agent report: when asked to,
// or by default when a coding agent runs goquality through a pipe and no
// other format was chosen.
func useAgentFormat(o options, args []string, stdout io.Writer) bool {
	if o.agent || o.json || flagSet(args, "agent") {
		return o.agent
	}
	return os.Getenv("CLAUDECODE") != "" && !isTerminal(stdout)
}

// rerunCommand is the command that re-runs goquality on the same target in
// agent mode.
func rerunCommand(o options) string {
	parts := []string{"goquality"}
	if o.cmd != "report" {
		parts = append(parts, o.cmd)
	}
	parts = append(parts, "--agent")
	if o.baseline != "" {
		parts = append(parts, "--baseline", o.baseline)
	}
	if o.dir != "." {
		parts = append(parts, "-C", o.dir)
	}
	return strings.Join(append(parts, o.args...), " ")
}

// target resolves the directory and package patterns to analyze. A single
// directory argument means "that directory, recursively".
func target(o options) (dir string, patterns []string) {
	if len(o.args) == 1 && !strings.Contains(o.args[0], "...") {
		if fi, err := os.Stat(o.args[0]); err == nil && fi.IsDir() {
			return o.args[0], nil
		}
	}
	return o.dir, o.args
}

// selectChecks returns the checks chosen by --only, --skip and
// --no-security.
func selectChecks(o options) ([]check.Check, error) {
	skip, err := parseChecks(o.skip)
	if err != nil {
		return nil, fmt.Errorf("--skip: %w", err)
	}
	only, err := parseChecks(o.only)
	if err != nil {
		return nil, fmt.Errorf("--only: %w", err)
	}
	return check.Filter(check.All(), only, skip, !o.noSecurity), nil
}

func runOptions(o options) check.Options {
	return check.Options{CyclomaticThreshold: o.cyclo, Coverage: o.cover}
}

func parseChecks(list string) (map[string]bool, error) {
	known := make(map[string]bool)
	for _, name := range checkNames() {
		known[name] = true
	}
	set := make(map[string]bool)
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, fmt.Errorf("unknown check %q (available: %s)", name, strings.Join(checkNames(), ", "))
		}
		set[name] = true
	}
	return set, nil
}

func render(w io.Writer, rep check.Report, o options) error {
	if o.agent {
		_, err := io.WriteString(w, report.Agent(rep, report.AgentOptions{
			MaxFindings: o.maxFinds,
			Command:     rerunCommand(o),
		}))
		return err
	}
	if o.json {
		data, err := report.JSON(rep)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	_, err := io.WriteString(w, report.Text(rep, report.TextOptions{
		Verbose: o.verbose,
		Color:   useColor(w, o),
	}))
	return err
}

func useColor(w io.Writer, o options) bool {
	return !o.noColor && isTerminal(w) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func exitCode(err error) int {
	if err != nil {
		return 2
	}
	return 0
}

func displayName(p *project.Project) string {
	if p.ModulePath != "" {
		return p.ModulePath
	}
	return p.Root
}

func checkNames() []string {
	var names []string
	for _, c := range check.All() {
		names = append(names, c.Name())
	}
	return names
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// status shows a transient progress line on a terminal.
type status struct {
	w       io.Writer
	enabled bool
}

func newStatus(w io.Writer, enabled bool) *status {
	return &status{w: w, enabled: enabled && isTerminal(w)}
}

func (s *status) set(msg string) {
	if s.enabled {
		_, _ = fmt.Fprintf(s.w, "\r\x1b[K%s...", msg)
	}
}

func (s *status) clear() {
	if s.enabled {
		_, _ = fmt.Fprint(s.w, "\r\x1b[K")
	}
}

// flagSet reports whether a boolean flag was given explicitly, including as
// --name=false.
func flagSet(args []string, name string) bool {
	for _, a := range args {
		a = strings.TrimLeft(a, "-")
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}
