---
name: gx-playwright-e2es
description: Rules for writing and reviewing gx's Playwright e2e flows (`webui/e2e/*.e2e.ts`). Covers waits before state reads, stale expectations and `-g` filters. Use when a ticket or diff touches a Playwright file.
---

# Playwright e2e rules

The generic test rules are in [gx-test-guidelines.md](../gx-test-guidelines.md). This skill adds the
Playwright-specific ones. Flow conventions are in `webui/e2e/CATALOG.md` and `.ai/index.md`
("Testing"). To drive the UI by hand, use [gx-ui-check](../gx-ui-check/SKILL.md), not this skill.

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
