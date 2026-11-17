---
name: gx-only-e2es
description: gx-only rules for Playwright e2e flows (`webui/e2e/*.e2e.ts`), the `playwrightguard` flow budget and `webui/e2e/CATALOG.md`. Load it before touching a Playwright file or writing a new flow.
---

# gx Playwright e2e rules

This skill is repo-local. It is not in the bundle. The generic Playwright rules (waits, state reads,
redirects, counts, flakes) are in the portable `gx-playwright-tests` skill, and the common test rules
are in `gx-test-guidelines`. Read those too. This skill adds what only gx has. Flow conventions are
in `webui/e2e/CATALOG.md` and `.ai/index.md` ("Testing"). To drive the UI by hand, use `gx-ui-check`,
not this skill.

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

## Before the first browser run

After you write or edit a flow, run `gx brief go-test ./playwrightguard` before any browser run.
It checks the rules above (state read before an `expect`, flow budget) in seconds. A browser run
takes 20-55 s, and `gx check` finds the same failures only at the end.

## Routes and labels

`webui/e2e/CATALOG.md` has a "Routes and labels" table. Check it before you guess a route or a label.

## Faking the platform

`isMac()` in `webui/src/platform.ts` reads `navigator.userAgentData.platform` first and
`navigator.platform` second. An init script that sets only `navigator.platform` fails on Chromium.
Set both:

```ts
await page.addInitScript(() => {
  Object.defineProperty(navigator, "platform", { get: () => "MacIntel" })
  Object.defineProperty(navigator, "userAgentData", { get: () => ({ platform: "macOS" }) })
})
```

## Stacked status roles and re-sorting sidebar rows

- Each row shows its own "Saved" element with `role="status"`, so the elements stack and
  `getByRole('status')` fails in strict mode. Scope the locator to one row first, for example
  `row.getByRole('status')`.
- A rename re-sorts the sidebar rows, so `.first()` can point at another chat afterwards. Locate a
  chat row by `a[href$="/agents/<id>"]`.

## `-g` patterns: use the brief command

Playwright matches `-g` against `[project] › file › title`, so a `^` anchor matches nothing and the
run passes with zero tests. Use `gx brief -C webui playwright <file> <flow numbers>`. It builds the
pattern. Check that the run reports the tests you named.

## gx forms of the generic rules

The reasons are in `gx-playwright-tests`. These are the gx cases and commands.

- **Redirects.** `goto(page, hash)` returns before the page settles. After a route that redirects
  (`#/settings`), wait for the final URL before the next `goto` (flow 22).
- **Boxes.** Several `boundingBox` reads after `setViewportSize` go in one `expect.poll` (flow 24).
- **Covered control.** A control covered by a fixed page is a product bug. Use no `force` and no
  wait (flows 152, 88, 117).
- **Sibling counts.** When a new menu item, tab or settings category lands, search `webui/e2e` for
  the counts of its siblings (`toHaveCount(4)`, the category list). Flows 72b, epics 2 and settings
  200 broke this way. Prefer `toContainText` of the named items over a count.
- **Rare flakes.** Run `gx brief -C webui playwright <file> <flow numbers> --repeat-each 20`. A
  repeat of 18 to 20 at load 3 or more shows a rare flake. One rerun proves nothing.
- **Timeouts.** Keep each `expect` timeout below the test timeout.

## macOS storm and burst flows need the cgo build

Storm and burst scenarios need the cgo `gx` build (`GX_E2E_GX_CGO`). Without it the watchdog cannot
read the process table on macOS, and the storm flow fails at a 150 s wait. Set the variable before
you debug that wait.

## Run a scratch copy of a flow

To try a flow without touching `webui/e2e`:

1. Copy `webui/e2e` to the scratchpad.
2. Symlink `src` and `node_modules` next to the copy.
3. Patch `global-setup.ts` for the new location.
4. Run with the cwd at the repo root. The `dev-mode` scenario builds a fake `pnpm` relative to it.

## Update the flow when the UI change was on purpose

**Why:** flow 22 kept expecting a transparent background after the amber pill was accepted. It failed
28 CI runs, deterministically.

**Fix:** when a ticket changes what a flow asserts, change that flow in the same ticket. Search
`webui/e2e/` for the old value.

## A failing flow: read `error-context.md` before anything else

**Why:** ticket 18 sat paused 4.9h on a question whose answer was in
`test-results/<flow>/error-context.md`: an earlier ticket had renamed the link text the flow looked
for. It had first built a clean worktree at HEAD, and the `node_modules` symlink broke `pnpm install`.
Tickets 14 and 45b also lost time on hand-built baselines.

**Fix:** on any Playwright failure, in this order:

1. Read the failing flow's `test-results/*/error-context.md` (page snapshot at the failure) with
   `rg -n` on its directory. It usually names the cause: a renamed label, a missing element.
2. Only then reproduce. Never build a clean worktree at HEAD or stash your diff to get a baseline.
   The rerun-once rule below is the whole baseline.
3. Never run `gx tickets ask` about a failure before step 1.

## An unrelated failing flow: rerun once, file it, stop

**Why:** ticket 14 reran a whole flow file about 25 times, with hand-built filters, on a failure
before its own steps. Ticket 23c ran `--repeat-each` at escalating counts on a flow that already
had a follow-up. A flake costs a minute per run and the reruns prove nothing new.

**Fix:** when a flow fails at a step before or outside your own steps:

1. Rerun that flow **once** on your change, with `gx brief -C webui playwright <file> <flow number>`.
2. If it fails the same way, file it with `gx-follow-up` (wording in `gx-flake-tickets`) and stop
   iterating on it. Do not add `--workers`, `--repeat-each` or a clean-worktree baseline.
3. If it passes, say so in one line. Call it flaky only with that evidence, not after a single rerun
   you did not compare.

Carry on with your own steps, and finish with `gx tickets set --check-failures` as `gx-failing-tests`
says.
