# goquality

`goquality` gives developers, coding agents and CI a fast summary of the
health of a Go codebase: correctness, maintainability, tests and security, in
one command and one binary.

```bash
go install github.com/iv-one/goquality/cmd/goquality@latest
cd your/go/project
goquality
```

```text
Go Quality  github.com/gojp/goreportcard

Grade ................................. A+
Score .............................. 94.0%

Project
  go version ...................... 1.24.2
  packages ............................. 7
  go files ............................ 38
  test files ........................... 7
  lines of code .................... 1,766
  lines of test code ................. 236
  functions .......................... 104
  dependencies ........................ 33
  direct dependencies .................. 9

Correctness
  build ............................. PASS
  go vet ............................ PASS
  staticcheck ....................... PASS
  errcheck ..................... 20 issues
  ineffassign ....................... PASS

Maintainability
  gofmt ............................. 100%
  complexity ......................... 99%
    average complexity ............... 3.5
    max complexity .................... 21
    most complex ............ check.GoTool
    functions > 15 ..................... 1
  misspell .......................... PASS
  license ........................ LICENSE

Tests
  tests ............................... 12
    packages tested .................. 3/7
    test files ......................... 7
  coverage ............. skipped (--cover)

Security
  vulnerabilities ...................... 0
    affected modules ................... 0
    imported, not called ............... 0
    required, not imported ............ 11
  security findings ................... 17
    high ............................... 0
    medium ............................ 11
    low ................................ 6

Overall
  issues .............................. 42
  time .............................. 3.0s
```

All analyzers are compiled into the binary. The only runtime dependency is
the `go` command; nothing else needs to be installed. Build goquality with a
Go toolchain at least as new as the projects you analyze (Go 1.26+). Its type
checker cannot parse language features newer than itself.

## Usage

```bash
goquality                  # analyze ./... in the current directory
goquality path/to/project  # analyze another directory, recursively
goquality ./cmd/... ./pkg/...
goquality --verbose        # list every finding with file:line
goquality --json           # machine-readable report
goquality --agent          # compact report for coding agents
goquality --cover          # also run tests and measure coverage
goquality --no-security    # skip govulncheck and gosec (e.g. offline)
goquality --skip misspell,gosec
goquality --only errcheck  # quick re-check of specific checks
goquality --min-score 90   # exit 1 if the score is below 90%
```

Every report ends with **next steps**: the checks ranked by how many score
points fixing them would add, with a hint for each and how many to fix for the
next grade:

```text
Next steps (A needs > 80%: fix 1-2)
  1.  +8.3%  go vet        2 issues in 2 files
             Fix the reported problems; go vet findings are almost always real bugs.
  2.  +6.2%  staticcheck   2 issues in 2 files
             Apply the suggested fixes; each rule is documented at https://staticcheck.dev/docs/checks/.
```

The same data is in the JSON report as `next_steps`.

Exit codes: `0` success, `1` score below `--min-score`, `2` usage or load error.

`--cover` is opt-in because it executes the project's tests.

## For coding agents

goquality is built to be run in a loop by an agent: *"run `goquality --cover`
and get it to A+"*. With `--agent`, the report is compact plain text:

- a one-line verdict
- which checks fail
- next steps ranked by score gain, with a fix hint each
- findings grouped by file, with suggested fixes, capped by `--max-findings`
  (default 50) with the highest-gain checks first
- a short footer that tells the agent to fix code rather than suppress it,
  and how to re-check

```text
goquality: grade B (70.3%), 13 issues, 2 suppressed | example.com/sample: 3 packages, 38 lines of code, 1 test

checks:
  fail: govet, staticcheck, errcheck, ineffassign, gofmt, misspell, nolint, license, tests, gosec
  skip: coverage (--cover)
  pass: build, complexity, govulncheck

next steps, by score gain (A needs > 80%: fix 1-2):
  1. govet +8.3% (2 issues in 2 files): Fix the reported problems; go vet findings are almost always real bugs.
  2. staticcheck +6.2% (2 issues in 2 files): Apply the suggested fixes; ...
  Re-check a single check: goquality --agent --only <check>

findings (13):
lib/lib.go
  12:9 staticcheck/SA6005: should use strings.EqualFold instead [fix: replace with strings.EqualFold]
main.go
  12:11 errcheck: unchecked error
  ...
```

The agent format is used automatically when goquality runs under Claude Code
(`CLAUDECODE` is set) and stdout is not a terminal. `--agent=false` turns it
off; `--json` takes precedence.

To make agents use goquality on their own, add a line like this to your
`CLAUDE.md` or `AGENTS.md`:

```markdown
Before finishing a change, run `goquality` and fix any new issues it reports.
```

## Guarding against regressions

`goquality check` answers "did this change make the project worse?". It
compares the working tree, uncommitted changes included, with a baseline and
exits 1 on a regression:

```bash
git fetch origin main
goquality check                          # baseline: merge base with origin's default branch
goquality check --baseline origin/release
goquality check --cover                  # also guard coverage (runs the tests twice)
```

```text
Baseline ............. origin/main@6dbe334
Current ..... working tree@6dbe334+changes
Score ...................... 93.1% → 92.0%

Checks
  errcheck .......... FAIL (1 new finding)
  gofmt ................... PASS (2 fixed)
  coverage .......... FAIL (78.4% → 78.1%)
  ...

Regressions
  errcheck: 1 new finding
      main.go:12:11 unchecked error
  coverage: 78.4% → 78.1%

FAILED: 2 checks regressed against origin/main
```

The baseline is the merge base of `HEAD` and the first of `origin/HEAD`,
`origin/main`, `origin/master`, `main` and `master` that exists, so work that
landed on main after you branched isn't counted against you. goquality exports
that revision with `git archive` to a temporary directory and analyzes it with
the same flags. The repository is left untouched, and git is only needed for
this.

The policy needs no configuration:

- **New findings fail.** Existing findings don't block unrelated work. A
  finding is identified by its check, file, rule and message (with numbers
  ignored), not by its line, so edits that move code don't create false
  positives. When a file has several identical findings, the content of the
  flagged line tells them apart.
- **For checks with no findings on either side, a falling score fails.** In
  practice this is coverage.
- Checks that didn't run on both sides are listed as not compared. A change
  in the number of suppressed findings is shown, so a regression hidden
  behind `//nolint` is visible.

`--json` and `--agent` work as for the report. Exit codes: `0` no
regressions, `1` regressions, `2` usage or load error.

The same comparison works on saved snapshots, for example to keep a
baseline outside git:

```bash
goquality collect -o base.json           # report + commit + settings, as JSON
goquality compare base.json current.json
goquality check --baseline base.json
```

In GitHub Actions, use the action from this repository. Check out full
history so the baseline exists:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
- uses: iv-one/goquality@v1
  with:
    args: --cover            # optional
```

The action builds goquality from its own source with the project's Go
toolchain, so the tool always understands the project's Go version, and then
runs `goquality check`. Set `command: ""` for the plain report (for example
with `args: --min-score 90`), and `working-directory` for a module in a
subdirectory. Without the action, the same steps are
`go install github.com/iv-one/goquality/cmd/goquality@latest` and
`goquality check`.

For pull requests into a branch other than the default, set
`args: --baseline origin/${{ github.base_ref }}`.

## Checks

| Section | Check | What it measures | Weight |
|---|---|---|---|
| Correctness | `build` | packages that fail to load or type-check | 0.10 |
| | `govet` | the `go vet` analyzer suite | 0.20 |
| | `staticcheck` | staticcheck's default-enabled SA, S and ST checks | 0.15 |
| | `errcheck` | unchecked errors | 0.10 |
| | `ineffassign` | assignments whose value is never used | 0.05 |
| Maintainability | `gofmt` | files that are not gofmt-formatted | 0.15 |
| | `complexity` | functions with cyclomatic complexity above 15 (gocyclo) | 0.10 |
| | `misspell` | common English misspellings | 0.02 |
| | `nolint` | `//nolint` directives without linter names or a reason | 0.05 |
| | `license` | a license file at the module root | 0.03 |
| Tests | `tests` | share of packages with tests; test counts | 0.05 |
| | `coverage` | statement coverage and test results (`--cover` only) | 0.10 |
| Security | `govulncheck` | known vulnerabilities from the Go vulnerability database | 0.10 |
| | `gosec` | security issues found by gosec, with gosec's severities | 0.10 |

The score is the weighted average of the checks that ran. For linters, a
check's score is the share of files with no findings, as in Go Report Card.
The grade uses Go Report Card's thresholds (A+ above 90%, A above 80%, ...).

The different kinds of security findings are scored differently:

- **govulncheck:** only vulnerabilities in code the project actually calls
  lower the score. Vulnerable packages that are imported but not called, and
  vulnerable modules that are only required, are reported but not scored.
- **gosec:** only HIGH and MEDIUM findings affect the score. G104 (unhandled
  errors) is excluded because it duplicates errcheck.

Generated files (with a `// Code generated ... DO NOT EDIT.` header) are
counted in the project statistics but excluded from all checks.

## Suppressing findings

goquality honors the directives other Go tools already use:

- `//nolint` and `//nolint:errcheck,gosec`, using golangci-lint linter names
  (`govet`, `staticcheck`, `gosimple`, `stylecheck`, `errcheck`,
  `ineffassign`, `gocyclo`, `misspell`, `gosec`) or rule IDs such as `SA6005`
  and `G401`
- staticcheck's `//lint:ignore SA6005 reason` and `//lint:file-ignore`
- gosec's `#nosec`
- gocyclo's `//gocyclo:ignore`

A directive applies to its own line. When the comment is alone on its line, it
also applies to the next line.

Suppressions are never free. The report counts suppressed findings, and the
`nolint` check flags any `//nolint` that doesn't name its linters or give a
reason (`//nolint:errcheck // cleanup is best-effort`). This keeps a
"make it A+" loop from being won by silencing linters.

## Development

Tasks use [Task](https://taskfile.dev); the Makefile has the same targets.

```bash
task install      # go install ./cmd/goquality
task lint         # go vet + gofmt
task test         # go test -race ./...
task test-short   # skip tests that need network access
task quality      # run goquality on itself (task quality -- -v for details)
task check        # goquality check on itself: no regressions against origin/main
go test ./internal/check -run TestRun -v
```

## Origins

goquality started as a fork of [Go Report Card](https://github.com/gojp/goreportcard)
by Herman Schaaf and Shawn Smith, which has been sunset. The hosted web service
is gone; the grading approach lives on as a local CLI. Licensed under the
Apache License 2.0.
