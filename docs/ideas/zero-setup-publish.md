# Zero-setup publishing that never breaks CI

## Problem Statement
How might we make publishing the quality report a single action step that
works on a repo's first run and never turns a pipeline red for reasons other
than code quality?

## Recommended Direction
`command: publish` in the action. On pushes to the default branch it collects
once, renders the badges, report.txt and report.json, and commits them to
quality-history with git plumbing, creating the branch if it is missing.
Elsewhere it skips with a notice. Push failures are warnings. The logic lives
in the action, not in README YAML, so fixes reach every @v0 user instead of
only those who copy the snippet again.

`goquality check` passes with a notice when there is nothing to compare
against: no default ref, or the module does not exist at the baseline (the
change that adds it). Never in a shallow clone, where a missing origin/main
means the history was not fetched, not that there is none.

## Key Assumptions to Validate
- [ ] A shallow clone without origin/main still exits 2 (Go test)
- [ ] The plumbing push works with the token actions/checkout persists, with
      and without an existing quality-history branch (our CI)
- [ ] Other files on quality-history survive a publish

## MVP Scope
In: action `command: publish` (default-branch guard, bootstrap, plumbing
commit, warning on push failure); a lax `check` for the no-baseline cases,
with tests; our CI uses `command: publish`; the README badges section becomes
a one-step install; v0.5.0.

## Not Doing (and Why)
- `continue-on-error`: hides real failures.
- A `goquality publish` subcommand: pushing is CI glue; git stays read-only in
  the binary.
- A configurable branch name or paths: no knobs until someone asks.
- Retrying push races: the next push to main publishes again.
- Leniency for an explicit `--baseline` that does not exist, or a baseline
  that fails to load: the user asked for that comparison.
