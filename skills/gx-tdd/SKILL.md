---
name: gx-tdd
description:
  Test-driven development against seams a ticket has already approved. Use when building a gx
  ticket's implementation test-first, mentions "red-green-refactor", or wants integration tests.
---

# gx Test-Driven Development

TDD is the red → green loop. This skill is the reference that makes that loop produce tests worth
keeping: what a good test is, where tests go, the anti-patterns, and the rules of the loop. Every
section applies on every cycle — consult them before and during the loop, not after.

When exploring the codebase, read `GLOSSARY.md` (if it exists) so test names and interface vocabulary
match the project's domain language, and respect ADRs in the area you're touching.

## What a good test is

Tests verify behavior through public interfaces, not implementation details. Code can change
entirely; tests shouldn't. A good test reads like a specification — "user can checkout with valid
cart" tells you exactly what capability exists — and survives refactors because it doesn't care
about internal structure.

See [tests.md](tests.md) for examples and [mocking.md](mocking.md) for mocking guidelines.

## Seams — where tests go

A **seam** is the public boundary you test at: the interface where you observe behavior without
reaching inside. Tests live at seams, never against internals.

**Test only at pre-agreed seams.** This skill runs unattended under ralph-loop, so "pre-agreed"
means agreed in the ticket, not negotiated live:

- **The ticket declares seams** (see `gx-to-tickets`'s `## Test seams` section) — use exactly those.
  An explicit `none — <rationale>` entry means write no automated test for that ticket; don't invent
  one anyway.
- **The ticket predates declared seams** (no `## Test seams` section at all) — you may proceed only
  when a minimal public seam is unambiguous from the acceptance criteria alone (e.g. "add a `Foo()`
  function returning X" has exactly one seam). If picking the seam requires a judgment call — several
  plausible boundaries, or the acceptance criteria don't pin down what's observable — that's material
  ambiguity: stop and run `gx tickets ask` (see
  [gx-local-tracker.md](../gx-local-tracker.md)) rather than guessing.
- **A live human is present** (not running under ralph-loop) — you may still ask "What's the public
  interface, and which seams should we test?" if the ticket's declared seams seem wrong or
  incomplete, and update the ticket with the corrected seams before writing tests against them.

**Every declared seam needs a real assertion.** A seam that is only partly covered (e.g. one of
three named backends, or one of three actions) counts as a gap. A weak check such as `cmd != nil`
or "did not error" does not cover a seam that names specific behavior. If you find a seam cannot be
observed (e.g. the code returns an opaque `tea.Cmd`), do not skip it and finish. Commit what is
green, then pause the ticket with `gx tickets ask` (see
[gx-local-tracker.md](../gx-local-tracker.md)): the question names the seam and why it
cannot be observed, so the ticket author can approve a different seam.

**Tests on skill text pin structure, not prose.** Collapse whitespace (newlines included) in both the
skill text and the expected phrase, so a reflow never breaks the test. In Go, reuse `normalizeSpace`
and `checkAnchors` from `skills/mock_rules_test.go`. Pin only structural anchors: headings, link
paths, selector names, numbers. Assert a phrase once, in one shared test file per skill area.

You can't test everything — agreeing the seams up front (in the ticket) is how testing effort goes
on the critical paths and complex logic instead of every edge case.

## Anti-patterns

- **Implementation-coupled** — mocks internal collaborators, tests private methods, or verifies
  through a side channel (querying the database instead of using the interface). The tell: the test
  breaks when you refactor but behavior hasn't changed.
- **Tautological** — the assertion recomputes the expected value the way the code does
  (`expect(add(a, b)).toBe(a + b)`, a snapshot derived by hand the same way, a constant asserted
  equal to itself), so it passes by construction and can never disagree with the code. Expected
  values must come from an independent source of truth — a known-good literal, a worked example, the
  spec.
- **Horizontal slicing** — writing all tests first, then all implementation. Bulk tests verify
  _imagined_ behavior: you test the _shape_ of things rather than user-facing behavior, the tests go
  insensitive to real changes, and you commit to test structure before understanding the
  implementation. Work in **vertical slices** instead — one test → one implementation → repeat, each
  test a **tracer bullet** that responds to what the last cycle taught you.

## Rules of the loop

- **Red before green.** Write the failing test first, then only enough code to pass it. Don't
  anticipate future tests or add speculative features.
- **One slice at a time.** One seam, one test, one minimal implementation per cycle.
- **Never leave the worktree root with a bare `cd`.** The shell keeps the new directory, so the
  next command runs in the wrong place. Run tests in a subshell, `(cd webui && npx vitest ...)`, or
  with absolute paths. Use the same worktree root for Bash and for Read/Edit paths.
- **Refactoring is not part of the loop.** It belongs to the review stage (`code-review`), not the
  red → green implementation cycle.
