---
name: gx-playwright-e2es
description: Rules for writing and reviewing gx's Playwright e2e flows (`webui/e2e/*.e2e.ts`). Covers whether a flow may be written at all, the flow budget, waits before state reads, stale expectations and `-g` filters. Use when a ticket or diff touches a Playwright file, or before writing a new e2e flow.
---

# Playwright e2e rules

The generic test rules are in [gx-test-guidelines.md](../gx-test-guidelines.md). This skill adds the
Playwright-specific ones. Flow conventions are in `webui/e2e/CATALOG.md` and `.ai/index.md`
("Testing"). To drive the UI by hand, use [gx-ui-check](../gx-ui-check/SKILL.md), not this skill.

## Write an e2e only when vitest or a Go test cannot cover it

Every flow starts its own fixture server and Chromium, so a flow costs about 6 seconds of CPU. The
suite grew to 310 flows and 35 minutes because render checks were written as flows. The default is
**no new flow**: write a vitest test (`webui/src`, jsdom) or a Go server test first.

A flow is allowed only if it needs at least one of these:

- a real server round trip: SSE, auth, cookies, restart, migration, the fixture's fake agents
- real browser layout: scroll, sticky, viewport or media-query behavior, focus order, real keyboard
- several pages or a reload-free navigation across the app

A flow is **not** allowed for:

- what a component renders for given props (text, badge, tone, order, empty state)
- a single control's behavior (disabled, toggle, validation message, keyboard shortcut in one input)
- a rule that lives in the server: assert it in a Go test on the server package
- a variant of an existing flow: add the assertion as one more step there

Rules when you do add a flow:

- **Extend before adding.** Look in `CATALOG.md` for a flow on the same scenario and add steps to
  it. A new flow needs a scenario no existing flow starts.
- **One flow per scenario and user journey,** not per assertion. Assert the whole journey in order.
- **Budget.** `playwrightguard` fails when the suite has more flows than `maxFlows`. Raise it only in
  a ticket whose title says why, and only after you tried moving the check to vitest. Lower it
  whenever you remove flows.
- **Say why in the PR or commit:** name the vitest or Go test you considered and what it cannot see.
- **No flow for a bug fix** unless the bug was in the server/browser seam. Pin the fix with a unit test.

## A state read follows an `expect` on the same locator

`evaluate`, `evaluateAll` and `getComputedStyle` read the page once, with no retry.

**Why:** a read right after navigation or a hash change sees the page before it renders. Flow 57
read `evaluateAll` right after setting the hash and got `-1` on a slow run. A read after
`toBeVisible()` on a different locator has the same problem.

**Fix:** put a web-first `expect(locator)` assertion on the same locator first, then read. Better,
replace the read with a web-first assertion (`toHaveCSS`, `toHaveCount`, `toHaveText`) that retries.

## Update the flow when the UI change was on purpose

**Why:** flow 22 kept expecting a transparent background after the amber pill was accepted. It failed
28 CI runs, deterministically.

**Fix:** when a ticket changes what a flow asserts, change that flow in the same ticket. Search
`webui/e2e/` for the old value.

## `-g` patterns match the full title

**Why:** Playwright matches `-g` against `[project] › file › title`. A `^` anchor matches nothing, so
the run passes with zero tests.

**Fix:** use `gx brief -C webui playwright <file> <flow numbers>`. It builds the pattern. Check that
the run reports the tests you named.

## Wait for the event, never for time

**Why:** a fixed `waitForTimeout` is too short on a loaded runner and too long everywhere else.

**Fix:** wait on a locator, a response or a URL (`expect(...)`, `waitForResponse`, `waitForURL`). See
the timing rules in the shared list.
