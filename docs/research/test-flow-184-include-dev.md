# Flow 184: why `dev.check()` ("Include dev") does not change state

## Answer

- Cause: a product bug. Toggling "Include dev" changes the stats query id (`range:project:includeDev`). `useFetched` has no cache for the new id, so `data` is `null` and `StatsPage` renders `<FullScreenLoader>` in place of the whole page body. The `WebPerformance` section, and the checkbox in it, unmount and mount again. Playwright's `check()` reads the state of the element it clicked. That element is gone, so it reports "did not change its state".
- Fix at the cause (product): while a refetch for a new query is in flight, keep showing the previous stats and mark them stale. Do not unmount the page body for a filter change. The loader stays only for the first load. The user also loses scroll position and focus today when they toggle, so this is a real fix for the user, not for the test.
- Fix for the flow (test): the reload added by ticket 18 needs `reloads: 1` in `start()`, because the fixture fails any flow that reloads without declaring it. No wait, retry or timeout change.
- Confidence: high for the unmount cause. It is observed, not inferred.

## Observed

Run with `gx brief -C webui playwright stats.e2e.ts 184`, with the ticket 18 fix applied (`location.hash = "#/stats"`, `page.reload()`, wait for the Web performance region). All edits reverted.

- Without a probe, `dev.check()` fails: "Clicking the checkbox did not change its state". The call log shows the click done and "navigations have finished".
- Probe: take `elementHandle()` of the checkbox, `click()`, wait 3 s, then read:
  - `isConnected` of the old element: `false`
  - old element `checked`: `false`
  - URL: `#/stats?range=30&dev=1`
  - fresh locator `isChecked()`: `true`
  - "Loading stats" text count after 3 s: 0
- Same result in 3 of 3 runs. The click works and the app is correct afterwards. Only the element identity changes.
- Code path: `useFetched.ts` returns `data = cached` when `loaded.id !== id`, and `cached` is `null` for an id never fetched. `StatsPage.tsx` (about line 396) renders `FullScreenLoader` when `!stats`.
- A second error shows in every run: "the page was reloaded more than the 0 allowed during the flow" (`fixture.ts` about line 534). `CATALOG.md` says a flow that reloads must declare `reloads`.

## Not resolved (hypotheses)

- One of four runs failed earlier, at the 60 s count poll (`expect.poll ... toBe(true)`, received `false`), before reaching `dev.check()`. I did not log which count stayed flat in that run. The other three runs read `[1,1,1,0,0]` before and `[2,2,2,1,1]` after. Treat the poll as possibly flaky until ticket 18's fix is repeated under load. Hypothesis: the INP sample is lost or late in some runs (ticket 18 saw the same).
- `uncheck()` failed with the same message in the probe runs. In those runs the toggle had already been clicked once, so the second click may hit the cached id and not unmount. I did not isolate this. After the product fix, both calls should pass without change.

## Spec changes

- Story 39: the Include dev step fails because the product unmounts the checkbox on a refetch. Fix the product to keep the previous stats during a refetch for a filter change, and declare `reloads: 1` in flow 184.
- Add a Vitest test: after clicking "Include dev" on a page with no cached stats for the new id, the checkbox element stays in the document until the new data arrives.

## Files

- `webui/e2e/stats.e2e.ts` (flow 184, `dev.check()` near line 160)
- `webui/e2e/fixture.ts` (`reloads` option, about lines 505 to 534)
- `webui/src/useFetched.ts` (`fresh`, `cached`, `data`)
- `webui/src/stats/StatsPage.tsx` (`FullScreenLoader` when `!stats`, about line 396)
- `webui/src/stats/stats.ts` (`useStats` query id)
