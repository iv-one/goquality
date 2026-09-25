# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project direction

`goquality` is a local CLI that summarizes the health of a Go project (correctness, maintainability, tests, security, project statistics) for developers, coding agents and CI. It started as a fork of Go Report Card (`gojp/goreportcard`); the web service, handlers, DB and repo-downloading code were removed and must not come back.

- Single self-contained binary: every analyzer is linked in as a library. The only runtime dependency is the `go` command. Don't add checks that shell out to separately installed tools.
- The default invocation (`goquality` in a module root) must be useful with zero configuration. Don't add config files or knobs speculatively.
- Prefer maintained ecosystem tools and APIs (`go/analysis`, `go/packages`, govulncheck, gosec) and keep their semantics: don't invent severities, and don't add exclusions beyond what the upstream tool does by default. Documented exception: gosec G104, which duplicates errcheck.

## Commands

Taskfile.yml and Makefile define the same tasks (keep them in sync; CI uses make).

```bash
task install                             # go install ./cmd/goquality
task lint                                # go vet + gofmt (testdata excluded)
task test                                # go test -race ./...
task test-short                          # skips TestGovulncheck (needs vuln.go.dev)
task quality                             # goquality on itself, --min-score 95 (also in CI); task quality -- -v
go test ./internal/check -run TestRun -v # single test
go run ./cmd/goquality -v internal/testdata/sample
```

## Architecture

The flow is `cmd/goquality` → `project.Load` → `check.Run` → `report.Text` or `report.JSON`.

- **`internal/project`** loads packages once with `go/packages` (`LoadAllSyntax`, `Tests: true`).
  - `selectRoots` analyzes each package in its test variant (`p [p.test]`) instead of the plain package, plus xtest packages, and drops synthesized `.test` mains. Because of this, a diagnostic can appear twice, and the analysis pass dedupes them.
  - `Project.Files` indexes the project's own files, with `Generated` detected by `ast.IsGenerated`. `Stats()` is cached and holds the descriptive numbers: LOC via `go/scanner`, test counts using `go test` naming rules, and deps from `go.mod` plus the loaded graph.
- **`internal/check`**:
  - **`Check` interface:** `Name`, `Category`, `Weight`, `Run(ctx, *Env) Result`. All checks are listed in `registry.go`, and adding a check means appending there. Simple checks use the `checkFunc` adapter.
  - **Concurrency:** the runner runs all checks concurrently and computes the weighted score. Only checks with a non-nil `Score` count, so skipped and errored checks don't.
  - **Shared analysis pass:** go/analysis-based checks are `analyzerCheck`s. The runner collects every check's analyzers up front, and the first `env.diagnostics` call runs them all in one `checker.Analyze` pass. Don't call `checker.Analyze` per check.
  - **Suppression:** each check must call `env.filter` or `env.suppressed` on its findings before computing its summary and score. The runner does not filter. Suppressed findings are counted per check (`Result.Suppressed`), and the `nolint` check flags vague directives, so a score can't be raised silently. Directives are parsed from AST comments in `suppress.go`, never raw text, which would match prose and strings. `nolintNames` maps check names to golangci-lint aliases.
  - **gosec** runs through `CheckRules`/`CheckAnalyzers` on the already-loaded packages, with one `gosec.Analyzer` per worker because instances aren't concurrency-safe. Its `Process` API re-runs `go list` per directory and was about 10x slower.
  - **govulncheck** runs in-process through `golang.org/x/vuln/scan` with `-json`. Its message types are internal to x/vuln, so `security.go` mirrors the fields it needs. Reachability comes from the first trace frame: function means called, package means imported, neither means only required.
  - **coverage** shells out to `go test -json -vet=off -coverprofile` and is opt-in (`--cover`) because it executes project code.
- **Next steps** (`check/next.go`): each unfinished check's exact gain, `weight*(1-score)/totalWeight`, ranked, with per-check hints from the `hints` map. A new check should get a hint there.
- **`internal/report`** renders three formats:
  - text: dotted `label .... value` lines, with colors only on a TTY
  - JSON: `check.Report` as-is
  - agent (`agent.go`): compact and token-conscious, with findings capped and prioritized by next-step gain. It's the default when `CLAUDECODE` is set and stdout isn't a TTY; see `useAgentFormat`.

  Anything an agent needs to act on (fix hints, suggested fixes, re-check commands) must show up in the agent format.
- **`internal/testdata/sample`** is a fixture module with one known issue per check. `TestRun` asserts exact findings (`file:line rule`), and `TestLoadStats` asserts exact stats, so changing the fixture means updating both. It is deliberately not gofmt-clean.
