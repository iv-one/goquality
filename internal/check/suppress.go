package check

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/iv-one/goquality/internal/project"
)

// nolintNames maps check names to the linter names used in //nolint
// directives (golangci-lint conventions).
var nolintNames = map[string][]string{
	"govet":       {"vet"},
	"staticcheck": {"staticcheck", "gosimple", "stylecheck"},
	"complexity":  {"gocyclo", "cyclop"},
}

var (
	nolintRe      = regexp.MustCompile(`^//\s*nolint(?::([\w,-]+))?(?:\s|$)`)
	lintIgnoreRe  = regexp.MustCompile(`^//lint:ignore\s+(\S+)`)
	lintFileIgnRe = regexp.MustCompile(`^//lint:file-ignore\s+(\S+)`)
)

// directive suppresses findings of the listed linters or rules; an empty
// list suppresses everything.
type directive []string

func (d directive) matches(check, rule string) bool {
	if len(d) == 0 {
		return true
	}
	names := append([]string{check}, nolintNames[check]...)
	for _, n := range d {
		if strings.EqualFold(n, rule) || strings.EqualFold(n, "all") {
			return true
		}
		for _, name := range names {
			if strings.EqualFold(n, name) {
				return true
			}
		}
	}
	return false
}

type fileDirectives struct {
	lines map[int][]directive
	file  []directive
	// vague lists lines with a //nolint that names no linter or gives no
	// reason, e.g. "//nolint" or "//nolint:errcheck" without "// why".
	vague []int
}

type suppressions struct {
	project *project.Project
	mu      sync.Mutex
	files   map[string]*fileDirectives
	count   map[string]int // suppressed findings per check
}

func (s *suppressions) init(p *project.Project) {
	s.project = p
	s.files = make(map[string]*fileDirectives)
	s.count = make(map[string]int)
}

func (s *suppressions) record(check string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count[check]++
}

func (s *suppressions) suppressedCount(check string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count[check]
}

func (s *suppressions) get(path string) *fileDirectives {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fd, ok := s.files[path]; ok {
		return fd
	}
	fd := parseDirectives(s.project, path)
	s.files[path] = fd
	return fd
}

// parseDirectives finds //nolint and staticcheck //lint:ignore directives
// among a file's comments. A directive applies to its own line and, when the
// comment is alone on its line, to the following line.
func parseDirectives(p *project.Project, path string) *fileDirectives {
	fd := &fileDirectives{lines: make(map[int][]directive)}
	f := p.File(path)
	if f == nil {
		return fd
	}
	src, err := f.ReadFile()
	if err != nil {
		return fd
	}
	lines := bytes.Split(src, []byte("\n"))
	for _, group := range f.Syntax.Comments {
		for _, c := range group.List {
			if !strings.Contains(c.Text, "lint") {
				continue
			}
			n := p.Fset.Position(c.Pos()).Line
			d, ok := fd.parse(c.Text, n)
			if !ok {
				continue
			}
			fd.lines[n] = append(fd.lines[n], d)
			if n <= len(lines) && bytes.HasPrefix(bytes.TrimSpace(lines[n-1]), []byte(c.Text)) {
				fd.lines[n+1] = append(fd.lines[n+1], d)
			}
		}
	}
	return fd
}

// parse interprets a comment as a directive. It reports whether the comment
// is a line directive; file-level directives are recorded directly.
func (fd *fileDirectives) parse(text string, line int) (directive, bool) {
	if m := nolintRe.FindStringSubmatchIndex(text); m != nil {
		var d directive
		if m[2] >= 0 {
			d = splitList(text[m[2]:m[3]])
		}
		if len(d) == 0 || !explained(text[m[1]:]) {
			fd.vague = append(fd.vague, line)
		}
		return d, true
	}
	if m := lintIgnoreRe.FindStringSubmatch(text); m != nil {
		return splitList(m[1]), true
	}
	if m := lintFileIgnRe.FindStringSubmatch(text); m != nil {
		fd.file = append(fd.file, splitList(m[1]))
	}
	return nil, false
}

// explained reports whether the text after a //nolint directive carries a
// "// reason" comment, as golangci-lint's nolintlint requires.
func explained(rest string) bool {
	_, reason, ok := strings.Cut(rest, "//")
	return ok && strings.TrimSpace(reason) != ""
}

func splitList(s string) directive {
	var d directive
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			d = append(d, part)
		}
	}
	return d
}

// filter drops findings suppressed by directives in the source. Checks call
// it before computing their score.
func (e *Env) filter(check string, findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if !e.suppressed(check, f) {
			out = append(out, f)
		}
	}
	return out
}

func (e *Env) suppressed(check string, f Finding) bool {
	if f.File == "" {
		return false
	}
	fd := e.suppress.get(filepath.Join(e.Project.Root, filepath.FromSlash(f.File)))
	for _, d := range append(fd.file, fd.lines[f.Line]...) {
		if d.matches(check, f.Rule) {
			e.suppress.record(check)
			return true
		}
	}
	return false
}

// Nolint reports //nolint directives that do not name the linters they
// silence or do not explain why. Suppressions are sometimes right, but each
// one should be deliberate and reviewable.
func Nolint() Check {
	return checkFunc{name: "nolint", category: Maintainability, weight: 0.05, run: func(_ context.Context, env *Env) Result {
		files := env.Project.SourceFiles(true)
		var findings []Finding
		for _, f := range files {
			for _, line := range env.suppress.get(f.Path).vague {
				findings = append(findings, Finding{
					File:    f.Rel,
					Line:    line,
					Message: "//nolint must name the linters and give a reason",
					Fix:     "use //nolint:<linter> // <reason>, or fix the underlying issue",
				})
			}
		}
		return Result{Findings: findings, Score: fileScore(len(files), findings)}
	}}
}
