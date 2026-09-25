# Agent mode

## Problem Statement
How might we make goquality the first thing a coding agent runs in a Go repo
and the last thing it runs before calling a task done, with output the agent
can act on without guessing?

The target workflow is concrete: *"run `goquality --cover` and get it to A+"*.

## Recommended Direction
Treat the report as a **gradient**, not a scoreboard. An agent needs to know
what to fix next, how much it's worth, how to fix it, and how to confirm it
worked, in as few tokens as possible.

So every report ends with **next steps**: checks ranked by exact score gain,
`weight × (1 − score) / total weight`, each with a fix hint and how many of
them reach the next grade. `--agent` renders the same information compactly,
with findings grouped by file, suggested fixes inline, and a cap that keeps
the highest-gain findings. It's the default under Claude Code when output is
piped.

The metric must stay honest once agents optimize for it. Suppressed findings
are counted and shown, and a `nolint` check flags directives that don't name
a linter or give a reason. An A+ earned by silencing linters is visible as
such.

## Key Assumptions to Validate
- [ ] Agents follow ranked next steps better than a raw findings list. Test
      by running the "get it to A+" prompt on a few real repos and comparing
      iterations and tokens against `-v` output.
- [ ] Counting suppressions is enough of a deterrent. Test by watching
      whether agents add `//nolint` with boilerplate reasons; if they do,
      consider penalizing suppressions in the score.
- [ ] 50 findings is a good default cap. Test on large repos (hundreds of
      issues) and check that agents narrow with `--only` instead of stalling.
- [ ] `CLAUDECODE` plus a non-TTY stdout is a reliable signal. Other agents
      set different variables; add them as they're confirmed.

## MVP Scope (shipped)
- Next steps with gains, hints and the grade target (text, JSON, agent)
- `--agent` format with `--max-findings`, auto-detected under Claude Code
- Suppression counts and the `nolint` check
- Suggested fixes surfaced on findings
- `--only` for fast re-checks

## Not Doing (and Why)
- **`--since <ref>`** (report only issues in changed code): valuable for
  "verify my change" and PR bots, but needs a baseline run of a second
  checkout. It stands alone, so it's next in line.
- **`goquality fix`** (apply suggested fixes): writes files, and the default
  is read-only. Agents can already apply the shown fixes themselves.
- **`goquality brief`** (orientation card and AGENTS.md snippet): nice for
  new repos, but the default report already carries the key stats.
- **MCP server**: shell output works with every agent today, so revisit once
  the output format has settled.
- **Penalizing suppressions in the score**: counts first; only penalize if
  agents are seen gaming with justified-looking reasons.

## Open Questions
- Which environment variables identify other agents (Codex, Cursor, Gemini
  CLI) reliably enough to auto-select `--agent`?
- Should coverage be on by default in agent mode, given that "get it to A+"
  usually means coverage too?
