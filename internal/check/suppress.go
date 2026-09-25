package check

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// nolintNames maps check names to the linter names used in //nolint
// directives (golangci-lint conventions).
var nolintNames = map[string][]string{
	"govet":       {"vet"},
	"staticcheck": {"staticcheck", "gosimple", "stylecheck"},
	"complexity":  {"gocyclo", "cyclop"},
}

var (
	nolintRe      = regexp.MustCompile(`//\s*nolint(?::([\w,-]+))?`)
	lintIgnoreRe  = regexp.MustCompile(`//lint:ignore\s+(\S+)`)
	lintFileIgnRe = regexp.MustCompile(`//lint:file-ignore\s+(\S+)`)
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
}

type suppressions struct {
	mu    sync.Mutex
	files map[string]*fileDirectives
}

func (s *suppressions) init() { s.files = make(map[string]*fileDirectives) }

func (s *suppressions) get(path string) *fileDirectives {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fd, ok := s.files[path]; ok {
		return fd
	}
	fd := parseDirectives(path)
	s.files[path] = fd
	return fd
}

// parseDirectives finds //nolint and staticcheck //lint:ignore directives. A
// directive applies to its own line and, when it is the only thing on its
// line, to the following line.
func parseDirectives(path string) *fileDirectives {
	fd := &fileDirectives{lines: make(map[int][]directive)}
	data, err := os.ReadFile(path) //nolint:gosec // path is a project source file
	if err != nil || !bytes.Contains(data, []byte("lint")) {
		return fd
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if !strings.Contains(line, "lint") {
			continue
		}
		var d directive
		found := false
		if m := nolintRe.FindStringSubmatch(line); m != nil {
			found = true
			d = splitList(m[1])
		} else if m := lintIgnoreRe.FindStringSubmatch(line); m != nil {
			found = true
			d = splitList(m[1])
		} else if m := lintFileIgnRe.FindStringSubmatch(line); m != nil {
			fd.file = append(fd.file, splitList(m[1]))
		}
		if !found {
			continue
		}
		fd.lines[n] = append(fd.lines[n], d)
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			fd.lines[n+1] = append(fd.lines[n+1], d)
		}
	}
	if sc.Err() != nil {
		return &fileDirectives{lines: make(map[int][]directive)}
	}
	return fd
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
	for _, d := range fd.file {
		if d.matches(check, f.Rule) {
			return true
		}
	}
	for _, d := range fd.lines[f.Line] {
		if d.matches(check, f.Rule) {
			return true
		}
	}
	return false
}
