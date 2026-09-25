// Command goquality reports the quality, maintainability, test and security
// health of a local Go project.
//
// Usage:
//
//	goquality [flags] [dir | packages]
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
	"github.com/iv-one/goquality/internal/project"
	"github.com/iv-one/goquality/internal/report"
)

const usage = `goquality reports the health of a Go project.

Usage:
  goquality [flags] [dir | packages]

With no arguments, goquality analyzes ./... in the current directory.
A single directory argument analyzes that directory recursively; otherwise
arguments are package patterns, as for "go build".

Flags:
`

type options struct {
	dir        string
	verbose    bool
	json       bool
	cover      bool
	noSecurity bool
	noColor    bool
	skip       string
	only       string
	minScore   float64
	cyclo      int
	version    bool
	args       []string // positional arguments
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes goquality and returns the process exit code: 0 on success,
// 1 when the score is below --min-score, 2 on usage or load errors.
func run(args []string, stdout, stderr io.Writer) int {
	logf := func(format string, a ...any) {
		_, _ = fmt.Fprintf(stderr, "goquality: "+format+"\n", a...)
	}

	o, err := parseFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	if o.version {
		_, err := fmt.Fprintln(stdout, "goquality", version())
		return exitCode(err)
	}

	skip, err := parseChecks(o.skip)
	if err != nil {
		logf("--skip: %v", err)
		return 2
	}
	only, err := parseChecks(o.only)
	if err != nil {
		logf("--only: %v", err)
		return 2
	}
	dir, patterns := target(o)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	status := newStatus(stderr, !o.json)
	status.set("loading packages")
	p, err := project.Load(ctx, dir, patterns)
	if err != nil {
		status.clear()
		logf("%v", err)
		return 2
	}

	status.set("analyzing " + displayName(p))
	checks := check.Filter(check.All(), only, skip, !o.noSecurity)
	rep := check.Run(ctx, p, checks, check.Options{
		CyclomaticThreshold: o.cyclo,
		Coverage:            o.cover,
	})
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

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("goquality", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.dir, "C", ".", "change to `dir` before running")
	fs.BoolVar(&o.verbose, "verbose", false, "list individual findings")
	fs.BoolVar(&o.verbose, "v", false, "shorthand for --verbose")
	fs.BoolVar(&o.json, "json", false, "print the report as JSON")
	fs.BoolVar(&o.cover, "cover", false, "run tests to measure coverage (executes project code)")
	fs.BoolVar(&o.noSecurity, "no-security", false, "skip security checks (govulncheck, gosec)")
	fs.BoolVar(&o.noColor, "no-color", false, "disable colored output")
	fs.StringVar(&o.skip, "skip", "", "comma-separated `checks` to skip, e.g. misspell,gosec")
	fs.StringVar(&o.only, "only", "", "comma-separated `checks` to run, e.g. errcheck,govet (for quick re-checks)")
	fs.Float64Var(&o.minScore, "min-score", 0, "exit with status 1 if the score is below `percent`")
	fs.IntVar(&o.cyclo, "cyclo-over", 15, "report functions with cyclomatic complexity above `n`")
	fs.BoolVar(&o.version, "version", false, "print version and exit")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(stderr, "\nChecks: %s\n", strings.Join(checkNames(), ", "))
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
		Color:   !o.noColor && isTerminal(w) && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb",
	}))
	return err
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
