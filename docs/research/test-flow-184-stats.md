# Flow 184 (stats): why web-performance counts stay `[1, 1, 1, 0, 0]`

## Answer

- Cause: the flow never does a hard load of a real route. `start()` ends with `page.goto(web_url)`, which lands on `/` with no hash. There is no `/` route, so the catch-all route in `webui/src/routes.tsx` (about line 284) calls `noteCatchAll()` and redirects to `/epics`. By design, that span "reports nothing" (see the comment above `catchAllRoute` and the vitest "the catch-all redirect queues nothing, on a load or later").
- The first span of a load is the hard span (`hardPending`). The catch-all span consumes it and queues nothing. Every later span is `route_ready`, so no `app_ready` is ever queued for that page.
- The refactor `45ab6e5d4` and `hardDropped` are not the cause (see below).
- Fix (test, at the cause): before `perfCounts()` is first read, load a real route as a hard load: `location.hash = "#/stats"` then `page.reload()`, then wait for the Stats page. `app_ready` is then queued from the hard span. No wait, retry or timeout change.
- Confidence: high for the `app_ready` cause. INP is observed to arrive with the fix. The step after the poll fails (see "Not resolved"), so the whole flow is not yet green.

## Observed

Temporary logging of `queueSample` / `flush` and of `/v1/web-performance` POSTs, run with `gx brief -C webui playwright stats.e2e.ts 184` (all edits reverted).

Unmodified flow:

- Queue contents at the hide flush (a `sendBeacon`, so `postData` is null in Playwright): `fcp(hard)`, `route_ready(stats)`, `route_ready(stats)`, `cls(hard)`, `lcp(hard)`.
- There is no `app_ready` and no `inp` call to `queueSample` at all (the log printed before the `enabled` check). So it is not the `enabled` gate and not `hardDropped`: LCP/FCP/CLS are queued as `tab: hard`, and `toSample` returns early on `hardDropped`.
- After the hide, a second POST carries one `route_ready(agents, trigger: click)`. That is the click on `[data-gx-action]`, which navigates to agents. The click produced no INP sample.
- Poll result: counts stay `[1, 1, 1, 0, 0]` for 60 s, so the flow fails.

Flow with `hash = "#/stats"` + `page.reload()` before `perfBefore`:

- Beacon at hide: `app_ready(hard, stats)`, `fcp`, `cls`, `lcp`.
- Second flush after the click: `route_ready(agents)` and `inp(hard)`.
- The count poll passed. So INP does fire for the synthetic click in this flow, and H2 (INP never fires) is rejected. It was only missing in the original because INP is reported on the hide after interactions; both times the same click is used, and with the original page it did not appear. Hypothesis, not tested: the INP in the original run was not reported in time or was lost with the first beacon; I did not isolate this.

Note on `BEFORE`: with the reload it read `[1, 1, 1, 0, 0]`, because the reload's `pagehide` flushed the first page's LCP/FCP/CLS. The poll compares to that baseline, so `app_ready` (0 to 1) and INP (0 to 1) are the two that must rise.

## Hypotheses checked

| Hypothesis from `test-playwright-root-causes.md` | Result |
| --- | --- |
| H1: hard-load `app_ready` dropped by `hardDropped` / `skipFirst` | `hardDropped` is not set (LCP/FCP/CLS pass the same check). The drop is the catch-all `skip`, not `skipFirst`. Partly right: the drop is real, the named cause is wrong. |
| H2: INP never fires | Rejected. INP is queued and counted once the page is a real hard load. |
| H3: refactor `45ab6e5d4` | The diff is a behavior-neutral move into `SpanTracker` (`begin()` split into `open`/`redirected`/`applyCatchAll`). The catch-all skip existed before it. Not tested at the parent commit. |

## Why it was red every time

- Flow 184 was added in `e96e3f439` (telemetry/17). The code review of telemetry/17 flagged it as unverified. It looks like it was never run green against a build where the load starts on `/`. Not verified in history: that is a hypothesis.
- Vitest covers the catch-all rule (`routes.test.tsx` "the catch-all redirect queues nothing, on a load or later") and `app_ready` from a hard load (`routeSpans.test.ts`, `routes.test.tsx` 1122). No vitest test is wrong. The flow's setup contradicts a documented rule.

## Not resolved

- After the poll passes, the flow fails at `dev.check()` ("Clicking the checkbox did not change its state", flow line about 160). The original run never reached this step. The error context shows the Web performance region rendered. I did not diagnose it. Candidates (hypotheses): the checkbox is controlled by the `dev` search param and the hash assignment `#/stats?range=30` races the page's own params; or including dev samples now changes the section. Next step: read `error-context.md` and `StatsPage.tsx` around line 333 with the reload in place.
- Test-only change, no product code change is needed. Do not edit `routeSpans.ts`.

## Files

- `webui/e2e/stats.e2e.ts` (flow 184, `perfCounts` block near line 114)
- `webui/e2e/fixture.ts` (`page.goto(out.web_url)`, around line 484)
- `webui/src/routes.tsx` (`catchAllRoute`)
- `webui/src/routeSpans.ts` (`SpanTracker.open`, `markReady`)
