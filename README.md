# goquality

[![Go Quality score](https://raw.githubusercontent.com/iv-one/goquality/quality-history/badges/score.svg)](https://github.com/iv-one/goquality/blob/quality-history/report.txt)
[![Go Quality grade](https://raw.githubusercontent.com/iv-one/goquality/quality-history/badges/grade.svg)](https://github.com/iv-one/goquality/blob/quality-history/report.txt)

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
Go toolchain at least as new as the projects you analyze (Go 1.27+). Its type
checker cannot parse language features newer than itself.

## Usage

```bash
goquality                  # analyze ./... in the current directory
goquality path/to/project  # analyze another directory, recursively
goquality ./cmd/... ./pkg/...
goquality --verbose        # list every finding with file:line
goquality --next           # end with next steps: what to fix first
goquality --json           # machine-readable report
goquality --agent          # compact report for coding agents
goquality --cover          # also run tests and measure coverage
goquality --no-security    # skip govulncheck and gosec (e.g. offline)
goquality --skip misspell,gosec
goquality --only errcheck  # quick re-check of specific checks
goquality --min-score 90   # exit 1 if the score is below 90%
goquality --from snap.json # print a snapshot from goquality collect, any format
```

With `--next`, the report ends with **next steps**: the checks ranked by how
many score points fixing them would add, with a hint for each and how many to
fix for the next grade:

```text
Next steps (A needs > 80%: fix 1-2)
  1.  +8.3%  go vet        2 issues in 2 files
             Fix the reported problems; go vet findings are almost always real bugs.
  2.  +6.2%  staticcheck   2 issues in 2 files
             Apply the suggested fixes; each rule is documented at https://staticcheck.dev/docs/checks/.
```

The same data is always in the JSON report as `next_steps`, and in the agent
format.

Exit codes: `0` success, `1` score below `--min-score`, `2` usage or load error.

`--cover` is opt-in because it executes the project's tests.

## For coding agents

goquality is built to be run in a loop by an agent: *"run `goquality --cover`
and get it to A+"*. With `--agent`, the report is compact plain text:

- a one-line verdict, with the goquality version, and a line saying so when
  blockers cap the score
- which checks fail
- next steps ranked by score gain, with a fix hint each
- findings grouped by file, with suggested fixes, capped by `--max-findings`
  (default 50) with the highest-gain checks first
- a short footer that tells the agent to fix code rather than suppress it,
  and how to re-check

```text
goquality v0.2.1: grade B (70.3%), 13 issues, 2 suppressed | example.com/sample: 3 packages, 38 lines of code, 1 test

checks:
  fail: govet, staticcheck, errcheck, ineffassign, gofmt, misspell, license, tests, gosec
  skip: coverage (--cover)
  info: nolint
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
- uses: iv-one/goquality@v0
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

## README badges

`goquality badges` writes two static SVG badges from the report's score and
grade: `score.svg` ("Go Quality | 93/100") and `grade.svg` ("Go Quality |
A+"). They are self-contained, with no scripts or remote assets, and the same
result always gives the same bytes.

```bash
goquality badges                         # analyze, write badges/score.svg and badges/grade.svg
goquality badges -o /tmp/b --cover       # include coverage in the score
goquality badges --from snapshot.json    # render a snapshot from goquality collect
```

No badge service is needed. CI renders the badges for the default branch and
commits them to a `quality-history` branch, and GitHub serves them from
there. Create the branch once:

```bash
git switch --orphan quality-history && git commit --allow-empty -m "Start quality history"
git push origin quality-history && git switch -
```

Then add a job that runs on pushes to main. It analyzes once, with
`goquality collect`, and renders the badges and the text report from that
snapshot, so all three agree. The action installs goquality, so later steps
can run it too:

```yaml
badges:
  if: github.ref == 'refs/heads/main'
  runs-on: ubuntu-latest
  permissions:
    contents: write
  concurrency: quality-history
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
    - uses: iv-one/goquality@v0
      with:
        command: collect
        args: -o ${{ runner.temp }}/report.json
    - run: |
        goquality="$(go env GOPATH)/bin/goquality"
        "$goquality" badges --from "$RUNNER_TEMP/report.json" -o "$RUNNER_TEMP/badges"
        "$goquality" --from "$RUNNER_TEMP/report.json" -v --agent=false > "$RUNNER_TEMP/report.txt"
    - uses: actions/checkout@v4
      with:
        ref: quality-history
        path: quality-history
    - working-directory: quality-history
      run: |
        mkdir -p badges && cp "$RUNNER_TEMP"/badges/*.svg badges/
        cp "$RUNNER_TEMP/report.txt" "$RUNNER_TEMP/report.json" .
        git add badges report.txt report.json
        git diff --cached --quiet && exit 0
        git -c user.name="github-actions[bot]" -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
          commit -m "Update badges and report for ${GITHUB_SHA::7}"
        git push
```

The job also saves the full report (`goquality -v`) as `report.txt`, so the
badges can link to the details behind the score, and the snapshot as
`report.json` for tools. Reference the badges from the README, replacing
`OWNER/REPO`:

```markdown
[![Go Quality score](https://raw.githubusercontent.com/OWNER/REPO/quality-history/badges/score.svg)](https://github.com/OWNER/REPO/blob/quality-history/report.txt)
[![Go Quality grade](https://raw.githubusercontent.com/OWNER/REPO/quality-history/badges/grade.svg)](https://github.com/OWNER/REPO/blob/quality-history/report.txt)
```

GitHub caches README images for a few minutes, so a new score can take a
moment to show up.

Dashboards, agents and other tools can read the latest result for the default
branch from
`https://raw.githubusercontent.com/OWNER/REPO/quality-history/report.json`.
It is the `goquality collect` snapshot: `report` is the same as
`goquality --json`, alongside the `commit`, `timestamp`, `goquality_version`
and the `settings` it was produced with. New fields may be added at any time;
renaming or removing one increments `schema`. The history of the branch is the
history of the report.

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
| | `nolint` | `//nolint` directives without linter names or a reason (informational) | — |
| | `license` | a license file at the module root | 0.03 |
| Tests | `tests` | share of packages with tests; test counts | 0.05 |
| | `coverage` | statement coverage and test results (`--cover` only) | 0.10 |
| Security | `govulncheck` | known vulnerabilities from the Go vulnerability database | 0.10 |
| | `gosec` | security issues found by gosec, with gosec's severities | 0.10 |

The score is the weighted average of the checks that ran. For linters, a
check's score is the share of files with no findings, as in Go Report Card.
The grade uses Go Report Card's thresholds (A+ above 90%, A above 80%, ...).

Generated files (with a `// Code generated ... DO NOT EDIT.` header) are
counted in the project statistics but excluded from all checks.

### How security affects the score

Security affects the score in two ways: as two weighted checks, like every
other check, and through **blockers**, which cap the whole score.

**Weighted checks.** `govulncheck` and `gosec` each weigh 0.10. Without
`--cover` all weights add up to 1.15, so each security check accounts for
about 8.7 points of the score (8.0 with `--cover`):

- **govulncheck:** each vulnerability in code the project actually calls
  takes 25% off the check, so four or more bring it to 0. Vulnerable packages
  that are imported but not called, and vulnerable modules that are only
  required, are reported as metrics but not scored.
- **gosec:** the check's score is the share of non-test files with no HIGH or
  MEDIUM finding. LOW findings are listed but not scored. G104 (unhandled
  errors) is excluded because it duplicates errcheck.

**Blockers.** A weighted average spreads one serious problem across the whole
project: a single `InsecureSkipVerify: true` in a 100-file project would cost
less than a tenth of a point. So some findings are blockers. While any
remains, the score is capped at **80%, so the grade is B at best**, however
clean the rest of the project is. Blockers are:

- vulnerabilities in code the project calls (govulncheck), and
- gosec findings with HIGH severity *and* HIGH confidence. In gosec v2.29
  these come from G108 (pprof endpoint exposed), G402 (`InsecureSkipVerify`,
  TLS versions or cipher suites that are too weak), G123 (TLS resumption
  bypassing `VerifyPeerCertificate`), G407 (hardcoded nonce or IV), G408
  (`ssh.PublicKeyCallback` misuse), and some findings of G119 (redirects
  forwarding sensitive headers) and G121 (CORS protection bypass).

Both the severity and the confidence are gosec's own. The confidence
requirement leaves out noisy rules such as G115 (integer overflow, HIGH
severity, MEDIUM confidence) and G101 (hardcoded credentials, LOW
confidence).

Because the cap applies to the score itself, the grade, the badges,
`--min-score` and the JSON report all agree. The report says when the score
is capped and what it would be without the cap. `-v` marks blocker findings,
and next steps (`--next`, and always in the agent format) list checks with
blockers first, with the gain of lifting the cap:

```text
Grade .................................. B
Score .............................. 80.0%
  capped at 80% by 1 blocker (97.3% without it)
...
      tls.go:7:48 [HIGH, blocker] TLS InsecureSkipVerify set to true. (confidence: HIGH) (G402)
```

In JSON, blocker findings have `"blocker": true`, and the report has
`blockers` and, when the cap lowered the score, `uncapped_score`.

A blocker can be suppressed like any finding, for example
`//nolint:gosec // pprof is only served on localhost`. Suppressions are
counted in the report (see [Suppressing findings](#suppressing-findings)).
With `--no-security` or `--skip gosec,govulncheck`, security checks do not
run, so they neither count toward the score nor cap it.

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

Suppressing a finding is your call, and it never lowers the score or fails
`goquality check`. A suppressed finding doesn't count against its check. To
keep suppressions visible, the report shows how many findings each check
suppressed, and the informational `nolint` check lists any `//nolint` that
doesn't name its linters or give a reason
(`//nolint:errcheck // cleanup is best-effort`).

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
