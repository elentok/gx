## Problem Statement

The person running gx wants to cut the web UI's bundle size and load time, but cannot tell whether a
change helps. Nothing measures how the web UI feels in the browser. The bundle has no ID of its own,
so a tab running an old bundle after an upgrade looks the same as a new one. The route-level bundle
split is done, and the next optimization has no before/after baseline.

## Solution

The browser measures real-user performance and reports it to the gx server on a best-effort path.
The server stores raw samples for 90 days under its private state directory. The Stats page gets an
account-wide **Web performance** section after Launch. It shows p75 values and sample counts for
hard loads and route changes, a daily trend, the slowest routes, a build-versus-previous-build strip,
and a collapsed **Why slow?** table. Below 20 samples a value dims and is never called good or poor.
Telemetry never delays rendering, navigation or sign-out.

## User Stories

1. As the person running gx, I want LCP, INP and CLS measured in the browser, so that I see the standard user-experience metrics.
2. As the person running gx, I want FCP measured on hard loads, so that I see the first visible paint.
3. As the person running gx, I want an `app_ready` time for the first usable gx screen, so that a loading screen does not count as loaded. A load that stops at sign-in, and a bfcache restore, record no app_ready.
4. As the person running gx, I want a `route_ready` time for each router navigation after the hard load, so that lazy chunks and primary data show up in the number.
5. As the person running gx, I want each page to say when its primary content has painted, so that readiness means usable content.
6. As the person running gx, I want a cache paint to count as ready and each sample to say `cache` or `network`, so that I can tell fast paints from fast fetches.
7. As the person running gx, I want an error or empty page to end the span with `outcome: error` or `empty`, so that failures are not lost or counted as slow.
8. As the person running gx, I want a navigation abandoned before ready to record nothing, so that it does not pollute the numbers.
9. As the person running gx, I want the navigation trigger (click, keyboard, history, programmatic) stored as its own field, so that triggers can be compared later.
10. As the person running gx, I want TTFB, blocking API latency and JS and CSS transfer bytes as diagnostics, so that a slow number points at a cause.
11. As the person running gx, I want each sample to carry the web build that actually ran, so that releases compare correctly even in a stale tab.
12. As the person running gx, I want a build named `web_version · short hash`, so that I can read it and still tell two builds of one version apart.
13. As the person running gx, I want samples from the Vite dev server tagged and hidden by default, so that my own development does not skew the numbers.
14. As the person running gx, I want an "Include dev" toggle, so that I can compare dev and prod on one machine.
15. As the person running gx, I want only normalized route names reported, so that no ticket address, query string, text or selector leaves the page.
16. As the person running gx, I want attribution kept only for poor INP (above 200 ms) and poor LCP (above 2.5 s), so that detail exists where it helps and nothing else is stored.
17. As the person running gx, I want no selectors, element text or raw URLs in any sample, so that the privacy rule holds.
18. As the person running gx, I want samples batched and flushed at 20 samples, after 10 idle seconds and when the page becomes hidden, so that nothing is lost on close and traffic stays small.
19. As the person running gx, I want the hidden flush to survive page close, so that the last samples of a visit arrive.
20. As the person running gx, I want the queue dropped at sign-out and sign-out never to wait on telemetry, so that telemetry cannot block leaving.
21. As the person running gx, I want a send that fails, answers non-2xx or comes back refused dropped without retry or error UI, so that telemetry is never a new failure.
22. As the person running gx, I want the endpoint to need my web login, so that strangers cannot write samples.
23. As the person running gx, I want oversized requests, rate floods and bad samples bounded by the server, so that a bug cannot fill the disk or hide good data.
24. As the person running gx, I want a resend of the same sample counted once, so that a retry or double flush does not double the data.
25. As the person running gx, I want samples kept for 90 days and no longer, so that storage stays bounded and matches the longest Stats range.
26. As the person running gx, I want a crash or restart to lose at most a torn last line, so that the data survives.
27. As the person running gx, I want a Web performance section on Stats after Launch, so that I find it with the other account-wide data.
28. As the person running gx, I want the section header to say it is account-wide and ignores the project filter, so that I do not expect it to change with the filter.
29. As the person running gx, I want the section to follow the 7/30/90-day range, so that it matches the rest of the page.
30. As the person running gx, I want a build strip naming the current and previous build with the change in LCP, INP and JS transfer, so that I see whether the last release helped.
31. As the person running gx, I want a Hard loads tab with LCP, INP, CLS, FCP and App ready cards, so that I see first-visit experience.
32. As the person running gx, I want a Route changes tab with Route ready, INP and CLS cards, so that I see in-app navigation.
33. As the person running gx, I want each card to show p75, sample count, a status pill, the change versus the preceding equal window (a dash when that window is not fully covered by stored data or either window has fewer than 20 samples; the server computes the change and returns null, which the page shows as a dash) and a one-line definition, so that I can read it cold.
34. As the person running gx, I want a status pill only where a standard threshold exists, so that App ready and Route ready are not judged by invented limits.
35. As the person running gx, I want a card below 20 samples to dim, show no pill and say "not enough samples", so that a tiny sample does not look like a verdict.
36. As the person running gx, I want a daily p75 trend with a one-metric picker, so that I never see milliseconds and CLS on one axis.
37. As the person running gx, I want trend days under 20 samples shaded, so that I can tell weak days from strong ones.
38. As the person running gx, I want the five slowest routes by p75 with counts, and "not enough" below 20 samples, so that I know where to look.
39. As the person running gx, I want a collapsed Why slow? table with TTFB, blocking API latency, JS and CSS transfer against the previous build, each row dimmed with "not enough" below 20 samples in the current build, so that I can act on a regression without trusting a tiny sample.
40. As the person running gx, I want poor-INP phase p75s (dimmed with "not enough" below 20 poor samples), slowest action names and the worst-frame chunk in that table, so that I see what made interactions slow.
41. As the person running gx, I want the tab and "Include dev" in the URL, so that a shared link shows the same view.
42. As the person running gx, I want a clear empty state when there are no samples, so that I do not mistake no data for a bug.
43. As a developer, I want canned web-performance data in the two fixture scenarios and the gallery entry below, so that the section can be seen and tested without real traffic.

## Implementation Decisions

**Browser measurement**

- Standard metrics come from the `web-vitals` attribution build, in one lazy chunk loaded after the app has started. `reportSoftNavs` is on for LCP, INP and CLS only. It does not drive readiness.
- Readiness comes from an explicit `useReady()` hook. Each page calls it when its primary content, or its empty or error state, has painted. The existing fetch hook feeds it. The browser's soft-navigation metrics are not used for readiness.
- A route span starts on every router navigation. `trigger` is a separate field. A span left before ready is dropped. An error or empty end is ready, with `outcome: error` or `outcome: empty`. An error over cached data is `ready`.
- The first span of a hard load is stored only as `app_ready`, measured from `navigationStart`. `route_ready` covers router navigations after the hard load, so the two never double count. A known redirect (such as the settings agents path) is one span for the final route. An unknown path that hits the catch-all reports nothing. Streaming pages (design, ticket, agents) are ready at the first rendered snapshot.
- Primary data is the page's main fetched result or first snapshot. Side panels, the sidebar epic list and later live updates do not count. Those fetches go through `apiFetchSide`, which records nothing, so they never set the span's blocking API latency.
- A hard load that shows the sign-in or setup screen records no `app_ready` and drops its hard-load metrics. The first span after a sign-in or setup screen records nothing. Telemetry queues samples only after the auth gate has passed; the "unknown" auth state counts as past the gate, and a send that then fails drops as usual.
- Every hard-load sample carries `navigation_type` (`navigate`, `reload`, `back-forward`, `back-forward-cache`), validated like the other enums.
- A bfcache restore stores its navigation type, records no `app_ready`, and its standard metrics count under Hard loads.
- Marks use `performance.mark`. A span stores only `route_ready - route_start`.
- A cache paint counts as ready. Each ready sample carries `data_source: cache | network`.
- Normalized routes: `epics`, `epic`, `archive`, `archive-epic`, `agents`, `stats`, `cleanup`, `schedules`, `schedule`, `tips`, `settings` (all sub-pages), `design`, `triage`, `ticket`. Paths that redirect report nothing.
- Diagnostics: TTFB, blocking API latency, JS and CSS transferred bytes. Same-origin script and stylesheet entries only. A zero transfer size means cached or unknown, not a zero-byte file.
- Attribution is kept only on poor INP (above 200 ms) and poor LCP (above 2.5 s). INP: input delay, processing, presentation delay, interaction type, a gx-owned action name, and the worst long animation frame (duration and allowlisted gx chunk name). LCP: TTFB, resource load delay, resource load duration, element render delay. All else the library returns is dropped. Good samples send the value only.
- Action names come from a small gx-owned allowlist set through a data attribute. Unknown targets report `other`. Chunk names outside the build-time allowlist report `other`. A browser without Long Animation Frames omits the worst-frame field and still sends the rest.
- `build_id` is the entry chunk hash, read at runtime from the entry script URL. The build passes the gx version to Vite as a `define` (`dev` for a plain build) and each sample carries `web_version`. The Vite dev server sends `env: dev` and `build_id: dev`. Fixtures and test servers run the built bundle and need no special handling.
- The session ID is random, made once per page load, held in memory only. A reload is a new session.

**Reporting path**

- `POST /v1/web-performance` is authenticated with the web login and sits behind the web mux's existing Origin and content-type guard. A signed-out call gets 401. The route is a logged-in web-only write (`WebWrite`, no socket handler, not public).
- Requirement: the hidden flush uses `sendBeacon` with a Blob typed `application/json`, so the request passes that guard. The fallback is `fetch` with `keepalive` and the same headers. The fallback is used when `sendBeacon` is missing, returns false or throws.
- Flush at 20 queued samples, after 10 seconds with no new sample, or when the document becomes `hidden`. Any refusal, non-2xx answer or failed send drops the batch with no retry. At sign-out the queue is dropped unsent and sign-out never waits.
- Limits: 50 samples and 32 KB per request. A larger request answers HTTP 200 with `refused: true` and reason `too-large`, and stores nothing. 30 requests per minute per client (web login plus remote address, no accounts); over that the answer is 200 with `refused: true` and reason `rate-limited`, and nothing is stored. Refusals follow ADR 0028 and replace the 413 and 429 wording of the design ticket. The `reason` strings are a public contract.
- The server drops only invalid samples and still answers success. Invalid: unknown metric or route name, negative or non-finite value, string over 64 characters, a zero timestamp, timestamp more than 1 day from server time. Other enumerated fields (outcome, trigger, data source, env) are validated the same way; an unknown value is invalid. Bad JSON or an unsupported schema version is 400 (ADR 0028).
- A sample's day is the server's local receive time, not the browser's timestamp.
- The wire shape is a schema version, the session, the build, and a list of samples. Field and enum names are the implementer's choice, inside the metric and field list above.
- Telemetry writes emit no server event. A page that shows the section reads on load and on changing range or "Include dev", never on a telemetry write.

**Storage**

- Raw samples, one JSON line each, in one file per server-local day under the private gx state directory, never the ticket store. Percentiles are exact, computed on read; the method (nearest rank or interpolated) is the implementer's choice and a test fixes it.
- Retention deletes whole files older than 90 days at server startup and once a day.
- Dedupe key is `session` plus sample `id`. Today's and yesterday's keys load at startup and stay in memory. A duplicate is dropped silently.
- Caps: 500 samples per session per day and 20,000 per day. Excess is dropped and the endpoint still answers success.
- Each batch is one append ending with a newline, private file mode. A torn last line is skipped on read, and the writer starts a new line if the file does not end with one. Past days' parsed samples are cached; only today's file is re-read.
- File and directory names are the implementer's choice. This is a new store beside the ticket markdown. ADR 0041 gets a one-line amendment: account-wide sections with no ticket source read their own bounded store.
- The read path caches compact per-day numeric columns, not full structs. A benchmark beside the existing stats benchmark runs at cap-sized data (20,000 samples a day for 90 days).
- No config key, no ticket or epic schema change and no migration step.

**Stats API and CLI parity**

- `web_performance` is a new section of the existing `GET /v1/stats` response. It follows the same 7/30/90-day range, ignores the project filter, and takes an `include_dev` flag. `gx server stats` prints it as a short text block and takes `--include-dev`, so the verb stays a thin client of the route.
- `POST /v1/web-performance` is a web-only route, like `POST /v1/client-errors`: a browser-only write with no CLI verb. It goes on the web-only list and is covered by the web-only route test. ADR 0028's amendment gets one line: "a browser-only write behind the web login may be web-only".
- The section returns, per metric and tab, p75, count and the change versus the preceding equal window (null is a dash; see the dash rule below); a daily p75 series with counts; the five slowest routes; the build strip; and the Why slow? rows. Status classes use the standard cutoffs: LCP 2.5 s / 4.0 s, INP 200 / 500 ms, CLS 0.1 / 0.25, FCP 1.8 s / 3.0 s.

**Stats page (approved mock, variant A)**

- The section is after Launch. Its header says it is account-wide and ignores the project filter. "Include dev" is a toggle in the header row, right-aligned beside the scope text, off by default. The scope line reads "All projects, ignores the project filter · p75 · samples from this browser build and others".
- Build strip: current build is the latest by first-seen time in the stored data, excluding dev unless "Include dev" is on. Previous is the build first seen just before it. Each build's p75 uses only its own samples in the selected range. It shows the change in LCP, INP and JS transfer. With no previous build it shows the current build and "no earlier build in range", with no change figures. A metric with fewer than 20 samples in either build shows a dash, not a percentage (the server returns null).
- Tabs: Hard loads (LCP, INP, CLS, FCP, App ready) and Route changes (Route ready, INP, CLS). A soft-navigation metric belongs to Route changes; a metric from a hard load or a browser without soft-navigation reporting belongs to Hard loads.
- Cards: p75, sample count, status pill (only with a standard threshold), change versus the preceding equal window, and a dim footer defining the metric in one line. The server computes the change and returns null for a dash when the preceding equal window is not covered or either window has fewer than 20 samples; the page renders null as a dash. A window is covered when the oldest stored day file is on or before the window's first day (so never for 90 days, since retention is 90 days). Below 20 samples a card keeps its value, dims, shows no pill and says "not enough samples".
- The daily p75 trend has a one-metric picker for LCP, INP, CLS, FCP, App ready and Route ready, never mixed units. Days under 20 samples are shaded. The slowest-routes table (five routes by p75 route ready, with count, from `route_ready` samples) and the trend do not change with the tab. A route row under 20 samples says "not enough".
- Collapsed **Why slow?**: TTFB, JS transferred and CSS transferred from hard-load samples, and blocking API latency (from both `app_ready` and `route_ready` samples), each compared with the previous build. A row dims and says "not enough" when the current build has fewer than 20 samples. The comparison shows a dash when either build has fewer than 20 samples or there is no previous build. Poor-INP phase p75s (input delay, processing, presentation delay) and the poor-sample count, from poor INP samples on both tabs; the phase p75s, the top action names and the worst-frame chunk dim and say "not enough" below 20 poor samples. Slowest interactions: the top 3 action names by poor-sample count. Worst-frame chunk: the most frequent one. LCP sub-parts are stored but not shown in v1.
- The Hard loads / Route changes tab and "Include dev" are in the URL beside `range` and `project`. The trend's metric picker is page-local and resets to LCP on reload.
- Empty state: with no samples in the range the section keeps its header and scope line, and the body reads "No web performance samples in this range." If dev samples exist while hidden, the line adds "Dev samples are hidden."
- The mock's route rows (including a `queue` row) are placeholder data. Real rows use the normalized route list above.

**Gallery and fixtures**

- Canned web-performance data for the gallery and the fixture scenarios is the implementer's choice. It must cover the three Mock states below. `web-performance` holds at least 14 days of samples with at least 20 per metric in each 7-day window.

## Testing Decisions

- A good test checks behavior from outside: the HTTP response and files on disk for the endpoint, the samples the browser queues and sends for a navigation, and what the page renders for a given payload. It does not test helper functions or component internals.
- Endpoint and storage (main seam): the web mux with a state directory in a temp dir, through the existing web handler test setup. Cases: 401 signed out, foreign Origin and wrong content type refused, `refused: true` with `too-large` over 50 samples and over 32 KB, `rate-limited` over 30 requests a minute (HTTP 200, nothing stored), each invalid-sample rule, success with partial drops, bad JSON and unsupported schema as 400, the day taken from receive time, dedupe across requests and across a restart, per-session and per-day caps, torn last line on read and on the next write, 90-day deletion at startup, and the stats section's p75, counts, change versus the preceding window, the dash rule (a partly covered 30-day window, a 90-day window, a window under 20 samples), 20-sample rule, build strip rule (first-seen order, no previous build, dev hidden and included), slowest routes and Why slow? rows. A benchmark beside the existing stats benchmark covers cap-sized data. Prior art: the Stats endpoint and web allowlist tests and the client-errors route test.
- Parity: the web-only route test covers the new route; the route-table test covers `gx server stats` printing the section.
- Browser measurement: Vitest with fake performance and web-vitals inputs. Cases: span start on each trigger, abandoned span records nothing, ready with cache, error and empty outcomes, first `route_ready` becomes `app_ready`, queue flush at 20, at idle and at hidden, the hidden flush uses a JSON Blob via `sendBeacon` and falls back to keepalive `fetch` when it is missing, returns false or throws, nothing queued before the auth gate or on the sign-in screen, bfcache restore, queue dropped at sign-out without waiting, failed, refused or non-2xx send not retried, attribution only on poor INP and LCP, `other` for unknown actions and chunks, build ID from the entry script URL, dev tagging. Prior art: `useFetched`, page cache and client-errors tests.
- Stats page: Vitest renders the section with a canned stats payload through the router, like the existing Stats page tests. Covers tabs, cards, pills, low-sample dimming, the change dash, trend shading, slowest routes, Why slow?, build strip with and without a previous build, empty state, and URL state for tab and "Include dev".
- Real browser: new steps in the Stats flow (flow 184 in `stats.e2e.ts`) run against a fixture with canned data, plus one step that loads a page, clicks, hides the page and checks the stored counts through `gx server stats --json`. Default is no new standalone flow.

## Verification

Flows run against a temporary gx server with the fixture (see the gx-ui-check procedure). Flow numbers are from the e2e flow catalog (the Stats flows are in `stats.e2e.ts`). A story entry that names a Mock state uses its look check and does not repeat it.

1. Run: flow 184 extended with a new step: read `gx server stats --json` counts, load a page, click, hide the page, and expect the LCP, FCP, CLS, `app_ready` and INP counts to rise. New: vitest with fake web-vitals.
2. Run: the same step. New: vitest that FCP is sent for a hard load only.
3. Run: Mock state `hard-loads` (App ready card). New: vitest that the first span of a hard load is stored as `app_ready` only; vitest that a load that stops at sign-in or setup records no `app_ready`, the first span after it records nothing, and nothing is queued before the auth gate; vitest that the "unknown" auth state counts as past the gate; vitest for a bfcache restore (no `app_ready`, `navigation_type` stored).
4. Run: Mock state `route-changes` (Route ready card). New: vitest for a span on each navigation after the hard load, a known redirect as one span, and the catch-all reporting nothing.
5. New: vitest per page that `useReady()` fires after primary content, and a test listing every route in the normalized list.
6. New: vitest for cache then network paint; endpoint test that `data_source` is stored.
7. New: vitest for `outcome: error` and `empty`, and error over cache as `ready`.
8. New: vitest for an abandoned span.
9. New: vitest for each trigger value.
10. Run: Mock state `hard-loads` (Why slow? rows). New: endpoint test for TTFB, API latency and byte aggregation.
11. Run: Mock state `hard-loads` (build strip). New: vitest for the build ID from a script URL; endpoint test that samples keep `build_id` and `web_version`.
12. Run: Mock state `hard-loads` (build strip label). New: vitest for the `web_version · short hash` label.
13. New: endpoint test and vitest for `env: dev` hidden by default.
14. New: Stats page vitest for the toggle; one added step in flow 184 turning "Include dev" on (the `web-performance` scenario has some `env: dev` samples).
15. New: endpoint test that a ticket address, query string or unknown path never reaches storage; vitest for route normalization.
16. New: vitest that attribution is sent only for poor INP and LCP; endpoint test that a good sample has none.
17. New: vitest and endpoint tests that no selector, element text or URL field exists.
18. New: vitest for the three flush triggers.
19. Run: the story 1 step in flow 184 (hide the page, counts rise). New: vitest for the hidden flush with `sendBeacon` and the keepalive fallback when it is missing, returns false or throws.
20. New: vitest that sign-out drops the queue and does not wait on a pending send.
21. New: vitest that a refusal (`too-large`, `rate-limited`), a non-2xx answer or a network failure drops the batch with no retry or error UI.
22. New: endpoint test for 401 signed out, and the Origin and content-type refusals.
23. New: endpoint tests for the `too-large` and `rate-limited` refusals (HTTP 200, `refused: true`, nothing stored) and invalid-sample drops. CLI and server verb check: `gx server stats --json` still answers after a request flood.
24. New: endpoint test for dedupe across requests and a restart.
25. New: storage test for 90-day deletion at startup and daily.
26. New: storage test for a torn last line.
27. Run: flow 184 (section is after Launch) with Mock states `hard-loads`, `route-changes` and `low-samples`. New: step in the Stats flow.
28. Run: Mock state `hard-loads` (section-head). New: vitest for the scope text and that changing the project filter does not change the section's values.
29. New: endpoint test and vitest for the 7/30/90 range. CLI check: `gx server stats --json` includes `web_performance` and its counts change with the range; `gx server stats --include-dev` adds dev samples to the counts and the plain text output prints a short web_performance block.
30. Run: Mock state `hard-loads` (build-compare); and the no-previous-build case on the `web-performance-low` scenario (a single build). New: vitest for that case and the dash rule.
31. Run: Mock state `hard-loads` (tabs, metric-cards).
32. Run: Mock state `route-changes` (tabs, metric-cards).
33. Run: Mock state `hard-loads`. New: vitest for the change versus the preceding window and that a null change renders as a dash (the rule itself is an endpoint test).
34. Run: Mock state `hard-loads` (App ready has no pill) and `route-changes` (Route ready has no pill).
35. Run: Mock state `low-samples`.
36. Run: Mock state `hard-loads` (trend-and-routes). New: vitest that the metric picker shows one metric at a time.
37. Run: Mock state `low-samples` (shaded days).
38. Run: Mock states `hard-loads` and `low-samples` (the "not enough" route row).
39. Run: Mock state `hard-loads` (why-slow collapsed, then expanded). New: vitest for dimmed "not enough" rows and the dash comparison; endpoint tests for the build comparison rules.
40. Run: Mock state `hard-loads` (why-slow, poor-INP block). New: vitest for dimming below 20 poor samples; endpoint test for the poor-sample count, top 3 actions and chunk rule.
41. New: vitest that tab and "Include dev" round-trip through the URL; the picker resets on reload.
42. New: vitest for the empty state and the dev-hidden line.
43. New: the two fixture scenarios and the gallery entry below.

CLI and server verb checks: `gx server stats --json` prints the `web_performance` section (stories 1, 29). The route-table and web-only route tests cover parity. Beyond those, no other verbs change.

New flows or scenarios to write (these become implementation tickets):

- Fixture scenario `web-performance` with at least 14 days of samples (at least 20 per metric in each 7-day window), enough canned samples for the `hard-loads` and `route-changes` states, two builds and some `env: dev` samples (stories 13, 14, 27 to 40).
- Fixture scenario `web-performance-low` with a single build, fewer than 20 samples for most cards and one route row with 6 samples (stories 30, 35, 37, 38).
- A gallery entry with the same canned data (story 43).
- New steps in flow 184 (`stats.e2e.ts`) for the section, "Include dev" and the tabs, and the load, click, hide step with the before and after counts. No new standalone flow.

**Mock states**

Note: the dash rule for change figures and the right-aligned "Include dev" toggle in the header row override the mock where it differs.

- `$(gx tickets root)/web-performance-telemetry/mocks/06-stats-section-prototype.html`, `hard-loads`, width 960, scenario `web-performance`, parts: section-head, build-compare, tabs, metric-cards (LCP, INP, CLS, FCP, App ready), trend-and-routes, why-slow.
- `$(gx tickets root)/web-performance-telemetry/mocks/06-stats-section-prototype.html`, `route-changes`, width 960, scenario `web-performance`, parts: section-head, build-compare, tabs, metric-cards (Route ready, INP, CLS), trend-and-routes, why-slow.
- `$(gx tickets root)/web-performance-telemetry/mocks/06-stats-section-prototype.html`, `low-samples`, width 960, scenario `web-performance-low`, parts: section-head, build-compare, tabs, metric-cards (dimmed, "not enough samples"), trend-and-routes (shaded days, one "not enough" route row), why-slow.

## Out of Scope

- The route-level bundle split, which is done, and further bundle optimizations. This work only measures them.
- Alerts or notifications on regressions.
- Sending telemetry anywhere outside the local gx server.
- Showing LCP sub-parts in Stats (stored only).
- A build filter beyond the current-versus-previous strip.
- Full resource waterfalls, memory, FPS, React render counts, long-task counts, `window.load` and DOMContentLoaded.

## Further Notes

- Research: `docs/research/web-performance-telemetry.md`.
- The service worker precaches under its own content hash, so a tab can run an old bundle after an upgrade. The entry-chunk `build_id` names the code that ran, which is why it is used instead of the server version.
- The 20-sample minimum applies everywhere: cards, change figures, trend days, route rows, Why slow? rows and poor-INP phases.
- Pill labels for the non-good bands are "needs improvement" and "poor".
