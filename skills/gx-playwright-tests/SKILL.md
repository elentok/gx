---
name: gx-playwright-tests
description: Rules for Playwright tests that stay green. Covers redirects, retrying assertions, covered controls, waits, test titles and flakes. Use when writing, fixing or reviewing Playwright tests.
---

# Playwright tests

These rules add the Playwright forms to the common rules in
[gx-test-guidelines](../gx-test-guidelines/SKILL.md). Read those first. Where the project's own e2e
docs set a rule, the project's rule wins.

## Read `error-context.md` first

**Why:** a long flow stops at its first stale line, so later stale lines stay invisible. One flow had
four stale lines and another had about a hundred more behind the first. The first failure is a
hypothesis, not the full list.

**Fix:** on any failure, read the failing test's `test-results/*/error-context.md` (the page snapshot at
the failure) before you reproduce or ask anyone. It usually names the cause, such as a renamed label
or a missing element. Keep each flow short enough that one failure does not hide the rest.

## Assert the destination after you navigate

**Why:** a hash change or `goto` is not done when the call returns. A route that redirects after its
data loads can overwrite the next navigation.

**Fix:** after navigating, assert the final URL or an element that only the destination page shows before the next
action.

## Read boxes together in `expect.poll`

**Why:** after a viewport change, several separate box reads each see a different frame, so the
numbers do not belong together.

**Fix:** read all the boxes inside one `expect.poll` callback. A poll is a retrying assertion. It does
not retry the test.

## Review renames by diff

**Why:** a search-and-replace for a renamed label can also hit a different control that has the same
old word.

**Fix:** review every change to e2e files by diff after a rename. Do not trust the search count.

## Visible and enabled is not clickable

**Why:** a fixed-position page or an overlay can cover a control that is in the accessibility tree.
The control passes the visible and enabled checks, and the click still fails.

**Fix:** read the call log. It names the element that intercepts the click. Report that as a product
bug. Do not wait it out and do not use `force`.

## A fixture exit and a SecurityError are one failure

**Why:** a fixture process that exits before it prints its output, followed by a `sessionStorage`
SecurityError, is one failure. The page was still at `about:blank`, and the SecurityError is only the
symptom.

**Fix:** read the line above the SecurityError, and fix the fixture start.

## Update the flow when the change was on purpose

**Why:** a flow that keeps expecting the old value fails on every run after the product change, and
fails the same way each time.

**Fix:** the commit that changes the product also updates the flow. Search the e2e folder for the old
text.

## Write an e2e only when a lower test cannot cover it

**Why:** every flow starts a browser and usually a server. A render check written as a flow costs
seconds of CPU and adds a flake source.

**Fix:** write an e2e only for a real server round trip, real browser layout or a journey over several
pages. Cover everything else with a unit or component test. Extend an existing flow before you add a
new one.

## A state read follows an `expect` on the same locator

**Why:** `evaluate`, `evaluateAll` and `getComputedStyle` read the page once, with no retry. A read
right after navigation sees the page before it renders.

**Fix:** put a web-first `expect` on the same locator first, then read. Better, replace the read with
a retrying assertion such as `toHaveCSS`, `toHaveCount` or `toHaveText`.

## `-g` patterns match the full title

**Why:** Playwright matches `-g` against the project, the file and the test title together. A `^`
anchor matches nothing, so the run passes with zero tests.

**Fix:** match a part of the title without an anchor. Check that the run reports the tests you named.

## Wait for the event, never for time

**Why:** a fixed `waitForTimeout` is too short on a loaded runner and too long everywhere else.

**Fix:** wait on a locator, a response or a URL, for example with `expect`, `waitForResponse` or
`waitForURL`.

## A new sibling changes the counts

**Why:** a new menu item, tab or settings category changes the count of its siblings. Old count
assertions then fail in flows you did not touch.

**Fix:** search the e2e folder for those counts (`toHaveCount(4)`, a category list) when you add the
item. Prefer `toContainText` on the named items over a count.

## Prove a rare flake under load

**Why:** a rare flake shows only under load. One rerun proves nothing. A longer timeout or a retry hides
the cause, and a retry pass does not count as a fix.

**Fix:** run with `--repeat-each` 18 to 20 at a machine load of 3 or more to reproduce it. Fix the
cause. Never add a retry or raise a timeout.

## Keep each `expect` timeout below its test timeout

**Why:** when the test timeout fires first, the failure names the test. When the `expect` timeout fires
first, it names the step.

**Fix:** set each `expect` timeout lower than the test timeout.

## A `check()` that did not change state can be an unmount

**Why:** `check()` can report that the element "did not change its state" when the element unmounted at
the click, for example when a loader replaced the page. The checkbox logic is not at fault.

**Fix:** read the call log before you change the test. If the element unmounted, it is a product bug.
