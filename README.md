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
goquality --cover          # also run tests and measure coverage
goquality --no-security    # skip govulncheck and gosec (e.g. offline)
goquality --skip misspell,gosec
goquality --min-score 90   # exit 1 if the score is below 90%
```

Exit codes: `0` success, `1` score below `--min-score`, `2` usage or load error.

`--cover` is opt-in because it executes the project's tests.

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

## Development

Tasks use [Task](https://taskfile.dev); the Makefile has the same targets.

```bash
task install      # go install ./cmd/goquality
task lint         # go vet + gofmt
task test         # go test -race ./...
task test-short   # skip tests that need network access
task quality      # run goquality on itself (task quality -- -v for details)
go test ./internal/check -run TestRun -v
```

## Origins

goquality started as a fork of [Go Report Card](https://github.com/gojp/goreportcard)
by Herman Schaaf and Shawn Smith, which has been sunset. The hosted web service
is gone; the grading approach lives on as a local CLI. Licensed under the
Apache License 2.0.
