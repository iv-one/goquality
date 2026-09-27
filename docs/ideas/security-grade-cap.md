# Security grade cap

## Problem Statement
How might we make a serious security problem visible in the score and grade,
without making the grade noisy or rewarding gosec's false positives?

Today gosec and govulncheck each weigh 0.10 of about 1.20, so security can
move the score by at most ~8 points per check. gosec scores the share of
files without HIGH or MEDIUM findings, so one `InsecureSkipVerify: true` in a
100-file project costs 0.08 points and the repo keeps its A+ badge. A
weighted average spreads a showstopper across every file; it cannot express
one.

## Recommended Direction
Some findings are **blockers**: while any exists, the overall score is capped
at 80%, so the grade is B at best. Blockers are:

- govulncheck vulnerabilities whose vulnerable symbol is called, and
- gosec findings with HIGH severity *and* HIGH confidence (G402 weak TLS,
  G108 exposed pprof, G123 TLS resumption bypass, G407 hardcoded nonce, G408
  SSH callback misuse, and some G119 and G121 findings).

The confidence filter is what makes this work: it leaves out G115 (integer
overflow, HIGH/MEDIUM) and G101 (hardcoded credentials, HIGH/LOW), the two
rules most often suppressed in practice. Both severity and confidence are
gosec's own, so no severities are invented; the cap value is goquality's
policy.

The cap applies to the score itself, so the grade, badges, `--min-score`, JSON
and the agent format all agree. It is always explained: the report says the
score is capped, from what, and by how many blockers. Next steps rank blocker
checks first and compute gains against the cap, so fixing a blocker shows its
true value (for example +17%) and agents fix it before typos.

## Key Assumptions to Validate
- [ ] Blockers are rare in healthy code. Run on 10-20 popular Go repos and
      count capped reports; above ~15% the cap is too blunt, most likely
      because of G108.
- [ ] Called vulnerabilities belong in the cap even though a stdlib
      vulnerability can drop a grade without a code change. The fix hint
      ("build with Go X or later") makes that actionable; watch for
      complaints.
- [ ] `#nosec G402` with a reason stays rare. Suppressions are already
      counted per check; revisit if capped repos turn A+ by suppression.

## MVP Scope
- `Finding.Blocker`, set by gosec (HIGH/HIGH) and govulncheck (called).
- `Report.Blockers` and `Report.UncappedScore`; `Score = min(score, 80)` when
  blocked.
- Next steps: blocker checks first, gains computed cumulatively with the cap.
- Text, agent and JSON output explain the cap and mark blocker findings.
- Fixture gets one G402 finding.

## Not Doing (and Why)
- Count-based decay for MEDIUM findings: for a CLI, G304 and G204 are the
  normal way to work; file-based scoring is the right model for them.
- Tiered caps (called vuln lower than G402): one number is easier to explain;
  tiers can be added later without breaking anything.
- A pass/fail security gate: badge readers would still see an A+.
- Configurable cap or blocker rules: zero configuration is a project rule.

## Open Questions
- Should a failing `build` also be a blocker? It already scores 0 on a
  weight of 0.10; revisit if broken repos keep high grades.
