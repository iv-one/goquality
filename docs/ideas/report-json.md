# Publish report.json next to report.txt

## Problem Statement
How might we give dashboards, agents and our own tooling a machine-readable
report for the default branch that always matches the badges and report.txt?

## Recommended Direction
Analyze once per push to main: `goquality collect -o report.json`, then
render the badges (`badges --from`) and report.txt (`goquality -v --from`)
from that snapshot, and commit all three to quality-history. The snapshot
already carries `schema`, `goquality_version`, `commit`, `timestamp` and
`settings`, which is what an outside consumer needs to trust the data, and it
doubles as a compare baseline.

This needs one small feature: `--from` on the plain report (text, JSON and
agent formats), reading the snapshot like `badges --from` does. It also fixes
the old job, which analyzed twice, so the badge and report.txt could come from
different runs (for example across a vulnerability database update).

## Key Assumptions to Validate
- [ ] Text rendered from a round-tripped snapshot is byte-identical to a live
      run. Tested on internal/testdata/sample with -v and --next.
- [ ] Flags that change the analysis are refused with `--from`, not silently
      ignored, for both the report and badges.
- [ ] The snapshot schema is stable enough to publish. Rule: adding fields is
      compatible; renaming or removing one bumps `schema`.

## MVP Scope
In: `--from` for the root command; tests; the CI badges job collects once and
renders three outputs; README snippet and a paragraph on report.json (URL,
schema rule); release v0.4.0, move v0, Marketplace release.

## Not Doing (and Why)
- Action outputs (score, grade, report path): a new interface to maintain;
  steps later in the same workflow can read the file with jq.
- report.json as a PR baseline: only correct when main is the merge base.
- Per-commit history files: the quality-history branch's git log already is
  the history.
- A JSON Schema document or separate API docs: a README paragraph is enough
  until someone asks.
