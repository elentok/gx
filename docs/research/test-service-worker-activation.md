# Why the service worker does not activate in the Playwright flows

Scope: login 102, dev-mode, and the PWA step of epics 2. Follow-up to Group C in `test-playwright-root-causes.md`.

## Answer

The install does not fail. It is slow: about 18 s on this machine, against a 10 s `expect.poll`. The precache in `webui/src/pwa/workerCore.ts` calls `fetch` for all 280 URLs and holds the responses with their bodies unread until every fetch has returned. Inside a service worker, Chrome then serves those responses at about three per 200 ms, so the install takes about 18 s. The flows poll `registration.active.state` for 10 s and see `none`.

Fix at the cause: read each body inside the `precache` map, so no response is held unread. One line:

```ts
return { url, response: new Response(await response.arrayBuffer(), response) }
```

Verified: 102 went from red (12 s, `none`) to green (4.2 s). Dev-mode passes. Epics 2 passes end to end (1.2 m) once its separate stale menu list is fixed (see "Verification").

## Findings (observed)

All runs used a `web-login` fixture and Chromium from the repo's `@playwright/test`, with `serviceWorkers: "allow"`.

1. Baseline red: `pnpm run e2e -g "102. Login" --workers 1` fails at `login.e2e.ts:50`, `Expected "activated", Received "none"`, after the 10 s poll.
2. The install succeeds, late. In 4 manual runs of a plain page load with no poll limit, the worker went `installing` for 18 to 19 s, then `activated`. No request failed. There were no `Network.loadingFailed` events and no `TypeError`.
3. Which URL fails: none did. I attached CDP `Network` to the service worker target. 280 requests, 0 failed. All 280 start within 3 ms of each other (page-relative 134 to 144 ms). Their finish times spread evenly over 18 s: groups of three finish at the same moment, one group about every 200 ms. They use only 3 connection ids.
4. The server is not the slow part. Fixture server log has nothing in that window. `curl` of the same 280 URLs (`xargs -P 280`) all return 200 in under 5 ms each (0.7 s total). One keep-alive `curl` of 60 URLs takes 16 ms.
5. The difference is where `fetch` runs and whether bodies are read. I ran the same 280 `fetch` calls with `cache: "reload"` four ways, in the page (service worker blocked) and inside the service worker (`worker.evaluate`):

   | variant | page | service worker |
   |---|---|---|
   | `Promise.all` of fetch, bodies unread (what `precache` does) | 39 ms | **17 890 ms** |
   | same, body read inside each map callback | 34 ms | 40 ms |
   | default cache, body read | 37 ms | 47 ms |
   | `no-store`, body read | 44 ms | 53 ms |

   (The first variant runs first in each context, so it is not a warm-up effect: the page's first variant is fast.) Only the unread-bodies case in the service worker is slow.
6. After the one-line fix, `login.e2e.ts` 102 passes in 4.2 s, the dev-mode flow passes, and `vitest run src/pwa` passes (41 tests).

## Verification

- 102: red before (`none` after 10 s), green after (4.2 s).
- Dev-mode: green after the fix. I did not run it on the unfixed code, so its red is taken from the earlier report.
- Epics 2: with the fix it still failed first at line 40, the stale menu list (`"Archive"` missing; the known epics 2 menu item in the earlier report). I added `"Archive"` temporarily to reach the PWA step: the flow then passed end to end (1.2 m). Not run unfixed.
- Temporary edits (the `workerCore.ts` line and the epics line) are reverted. `git diff` shows only this report. `webui/dist` is git-ignored and was rebuilt by the e2e global setup, so it now holds the fixed build; that is harmless.

## Hypotheses (not proven)

- H1: Chrome keeps an HTTP/1.1 connection busy while a response body is unread, and the worker's body pipes back-pressure, so 280 held responses on about 3 connections drain only as Chrome's internal timer releases them (the 200 ms step). I saw the pattern (3 connections, 3 finishes per 200 ms) but did not find the Chromium code that owns it. The page does not show it, so the page and the worker use different body-handling paths.
- H2: the `TypeError: Failed to fetch` at `sw.js:125` in the earlier report came from teardown. The test fails at 10 s and the context closes while the install is still waiting on unread bodies, which cancels them. I never saw the error with the page left open for 20 s. I did not capture it at teardown.
- H3: on a slower or busier machine (3 workers) the install takes longer than 18 s, so the flows would stay red even with a longer poll. Not measured. This is why a longer timeout is not the fix.

## Why the earlier tries missed it

The earlier note says "unread bodies held in an array" was ruled out. That test ran in the page, where the same pattern takes 39 ms (finding 5). It needs to run inside the service worker.

## Fix list

1. `webui/src/pwa/workerCore.ts` `precache`: read the body in the map callback (line above). Memory cost: the whole dist (about 14 MB) buffered once, which the code already did by holding every response.
2. Add a Vitest case in `workerCore.test.ts` that the precache reads every body before `caches.open` (a fake response whose `arrayBuffer` is spied). A Vitest test cannot show the Chrome stall, so keep it as a guard on the rule, not as proof.
3. Do not raise the 10 s poll. Epics 2 also needs the stale `"Archive"` menu entry (separate item in the earlier report, test-only).
4. Optional hardening, not needed for the cause: bound concurrency. Reading bodies already makes the install take about 40 ms in the worker.

Scratch scripts are in the session scratchpad (`run3.mjs` CDP timing, `run4.mjs` four-variant experiment) and are not part of the repo.
