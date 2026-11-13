# Web UI performance telemetry

Question: what should gx measure in its React/Vite web UI, how should the browser report it to the
gx server, and what belongs on the existing Stats page?

Sources: current browser and library documentation read on 2026-11-13, plus the gx code at this
branch. External claims below link to the source that owns the API or recommendation.

## Recommendation

Ship a small real-user monitoring (RUM) set in two layers:

1. **Experience:** LCP, INP and CLS, plus FCP, initial app-ready time and SPA route-ready time.
2. **Diagnosis:** TTFB, the slowest route-blocking API request, initial JavaScript/CSS transfer
   bytes, and INP/long-frame attribution only when a sample is slow.

The Stats page should lead with p75 experience metrics and sample count, split hard loads from SPA
route changes, and show a daily trend. It should not lead with `window.load`, DOMContentLoaded,
individual resource waterfalls or a generic long-task count: those are implementation signals, not
the experience gx is trying to improve.

Use the `web-vitals` package rather than implementing the standards by hand. Its standard build is
about 3 KB Brotli, matches Chrome's methodology, supports soft-navigation reports, and can be loaded
as a separate lazy chunk. The attribution build costs about another 1.5 KB Brotli and is useful only
when gx will store and display the diagnostic fields
([web-vitals README](https://github.com/GoogleChrome/web-vitals/blob/main/README.md)). Start with the
standard build and add a small, explicit set of attribution fields later.

## What to measure

### Primary experience metrics

| Metric | Scope | Purpose | Display |
| --- | --- | --- | --- |
| Largest Contentful Paint (LCP) | Hard and supported soft navigations | Loading of the main visible content; the Core Web Vitals "good" threshold is 2.5 s | p75, poor share, daily p75 |
| Interaction to Next Paint (INP) | Page/soft-navigation lifetime | Worst representative interaction latency, including input delay, handler work and presentation delay; good is 200 ms | p75, poor share, daily p75 |
| Cumulative Layout Shift (CLS) | Page/soft-navigation lifetime | Unexpected visual movement; good is 0.1 or less | p75, poor share, daily p75 |
| First Contentful Paint (FCP) | Hard navigation | First visible text/image/canvas; useful for a client-rendered shell even though it is not a Core Web Vital; good is 1.8 s | p75 and daily p75 |
| App ready | Hard navigation, custom | Navigation start until gx has painted the shell and the initial route's usable content, not merely a spinner | p50/p75/p95 and daily p75 |
| Route ready | SPA navigation, custom | User navigation start until the destination route has painted usable content; catches lazy-chunk and data waterfalls across browsers | p50/p75/p95 by normalized route |

LCP, INP and CLS cover loading, responsiveness and visual stability. Google recommends judging them
at the 75th percentile, with good/poor cutoffs of 2.5/4.0 s for LCP, 200/500 ms for INP and
0.1/0.25 for CLS
([threshold rationale](https://web.dev/articles/defining-core-web-vitals-thresholds/)). FCP is the first
content paint, not completion of the main content, and its good/poor cutoffs are 1.8/3.0 s
([FCP guide](https://web.dev/articles/fcp)).

"Time to first render" should therefore not be a new React-specific metric. FCP already measures the
first browser paint consistently, while LCP measures when the main visible content appears. The one
gx-specific gap is **usable app content**, so `app_ready_ms` should end only after the initial route's
real content has committed and painted. Keep a separate shell-paint mark during development if useful,
but do not put it on Stats unless it leads to a distinct decision.

### Diagnostic dimensions and metrics

Collect these with each primary sample or route span:

- `navigation_kind`: hard navigate, reload, back/forward, bfcache, restore or soft navigation.
- `route`: a low-cardinality route template such as `ticket`, `stats`, `agents` or `epic`, never the
  URL, query, hash, project, epic or ticket identifier.
- `build`: the server/build version. Comparing before and after an optimization is unreliable when
  old assets remain cached unless samples carry their actual deployed version
  ([field-measurement guidance](https://web.dev/articles/vitals-field-measurement-best-practices)).
- `api_blocking_ms`: duration and endpoint name of the slowest API call that blocked app/route ready.
  This separates server/network delay from lazy import and render delay.
- `js_transfer_bytes`, `js_encoded_bytes`, `css_transfer_bytes` and `css_encoded_bytes` for the
  initial view. Resource Timing exposes timings and sizes for fetched resources
  ([Resource Timing](https://www.w3.org/TR/resource-timing/)). Use only same-origin script and
  stylesheet entries, and label a zero transfer size as cached/unknown rather than "zero-byte file".
- `ttfb_ms` from `PerformanceNavigationTiming.responseStart`. TTFB precedes every paint metric and is
  a useful diagnosis for gx's initial HTML, but it is not a Core Web Vital and should not be the main
  score. The rough good threshold is 800 ms
  ([TTFB guidance](https://web.dev/articles/optimize-ttfb)).

For poor INP samples, keep the attribution categories from the library: input delay, processing
duration, presentation delay, interaction type, and a gx-owned action name. Do not store arbitrary
DOM selectors or element text. Long Animation Frames provide better script/render attribution than
generic Long Tasks, and `web-vitals` can associate intersecting frames with INP. Chrome recommends
that association for diagnosing slow interactions
([Long Animation Frames](https://developer.chrome.com/docs/web-platform/long-animation-frames)).
Store detailed frame attribution only for slow INP samples, capped to the worst frame and a small
allowlist of gx chunk names. Other browsers may not expose it.

Do **not** collect these in v1:

- Full resource waterfalls or every resource name. Official field guidance explicitly warns against
  collecting available performance data without a use for it
  ([field-measurement guidance](https://web.dev/articles/vitals-field-measurement-best-practices)).
- Raw URLs, selectors, element text, stack traces, project/ticket names or interaction coordinates.
- Memory use, FPS averages, React render counts or long-task counts. They are hard to interpret on a
  Stats trend without an attached user-visible regression.
- DOMContentLoaded and `window.load` as headline metrics. Navigation Timing defines them precisely,
  but a client-rendered application can still be showing a loader after both events
  ([Navigation Timing Level 2](https://www.w3.org/TR/navigation-timing-2/)).

## SPA route measurement

gx uses React Router and now lazy-loads most routes, so initial navigation and later route changes are
different products. Report them separately.

Chrome 151 can measure Core Web Vitals for soft navigations, and current `web-vitals` exposes
`reportSoftNavs` plus the navigation URL/id/start time. Other browser engines still measure Core Web
Vitals only for full page loads
([SPA and Core Web Vitals FAQ](https://web.dev/articles/vitals-spa-faq)). Enable that support when
available, but keep gx's custom `route_ready_ms` for cross-browser coverage and for the gx-specific
definition of usable content.

A route span should have three marks:

1. `route_start`: the click/keyboard/history navigation begins.
2. `route_shell_paint`: the destination boundary paints, even if it is loading.
3. `route_ready`: the destination's primary data or empty/error state has painted.

Only `route_ready - route_start` needs long-term storage. The middle mark and the slowest blocking API
duration explain whether a slow result came from route code/rendering or data. End the span on error as
an error state rather than dropping it, and carry `outcome: ready | empty | error`.

The browser's soft-navigation heuristic requires a user interaction, DOM modification/paint and URL
update. It is intentionally framework-independent
([Chrome Soft Navigation API](https://developer.chrome.com/blog/new-soft-navigations-origin-trial)).
gx's explicit route span should cover programmatic and back/forward navigation too, and record the
trigger separately.

## Reporting path

Add an authenticated, same-origin web write such as `POST /v1/web-performance`. It should be best
effort and must never delay rendering, navigation or sign-out:

- Lazy-load and install measurement after the application has started. The performance APIs retain
  buffered entries, so measurement does not have to block startup
  ([field-measurement guidance](https://web.dev/articles/vitals-field-measurement-best-practices)).
- Queue compact samples in memory. Flush a bounded batch after idle time, at a small batch limit, and
  when `visibilitychange` reaches `hidden`.
- Use `navigator.sendBeacon()` for the final small POST, with `fetch(..., {keepalive: true})` as the
  fallback. `visibilitychange` is more reliable than unload and does not harm the back/forward cache
  ([Beacon API](https://developer.mozilla.org/en-US/docs/Web/API/Navigator/sendBeacon)).
- Drop on failure. Never retry indefinitely, show an error, or call the sign-out path because
  telemetry failed.
- Accept only a small body and a fixed schema. Reject unknown metric names, non-finite/negative
  values, overlong strings, excess samples and implausible timestamps. Apply a per-client rate cap.

The existing `clientErrors.ts` demonstrates a fire-and-forget browser-to-server path, but performance
telemetry should be authenticated and persisted rather than public and logged. Keep it in a separate
module and endpoint so error reporting remains available on the login page.

A compact sample is enough:

```json
{
  "schema": 1,
  "session": "random-per-tab-id",
  "build": "server-build-id",
  "samples": [
    {
      "at": "2026-11-13T12:34:56Z",
      "id": "v4-123456789",
      "metric": "route_ready",
      "value": 184,
      "unit": "ms",
      "route": "stats",
      "navigation_kind": "soft-navigation",
      "outcome": "ready",
      "api_blocking_ms": 91
    }
  ]
}
```

Use the metric's library-generated id plus the random per-tab session id for deduplication. The
session id must not survive a tab or become a user identifier.

## Server storage and aggregation

This is machine-local, low-volume telemetry, so sample every gx page view rather than probabilistically
sampling. Bound cost at the server instead:

- Store validated samples under the private gx state directory, not the ticket store and not Git.
- Retain 90 days, matching the longest Stats range. Daily append files or one bounded rolling store
  both work; writes need to survive a server restart and use gx's atomic/private file helpers.
- Cap duplicates and samples per session/day. Keep primary samples plus poor-sample attribution, not
  raw PerformanceEntry objects.
- Aggregate by local day, metric, hard/soft navigation, normalized route and build. Never group by a
  raw identifier.
- Return p50, p75, p95, count and threshold shares. Use p75 as the headline because that is how Core
  Web Vitals are judged, but always show `n`; gx's single-user sample sizes can be too small to make a
  percentile stable.

Web performance is account/machine-wide. The existing Stats project selector should not change it,
just as the current limit-burn section is account-wide. The server can add a `web_performance` section
to `GET /v1/stats`; the Stats page already supports the same 7/30/90-day windows.

## Stats page shape

Add a **Web performance** section after Launch:

- Six summary cards: LCP, INP, CLS, FCP, App ready and Route ready. Each shows p75, sample count, change
  versus the preceding equal-length window, and the standard good/needs-improvement/poor status where
  a standard threshold exists.
- A daily p75 trend with metric selection. Do not combine milliseconds and CLS on one axis.
- Hard load / route change tabs. The route-change view can list the five slowest normalized routes by
  p75 with count.
- A collapsed "Why slow?" table: TTFB, blocking API, JS transfer, CSS transfer, and poor-INP phase
  breakdown. This is where an optimization becomes actionable.
- Build filter/comparison only after enough builds coexist in the selected window. Always retain the
  build dimension in storage from day one.

Do not label a metric "good" from one or two samples. Show "not enough samples" below a chosen minimum
while still displaying the values. For gx's scale, a minimum of 20 samples is a reasonable product
choice, not a web standard; the UI should make that policy explicit.

## Suggested rollout

1. **Baseline:** standard `web-vitals` LCP/INP/CLS/FCP/TTFB, `app_ready`, `route_ready`, build, route,
   navigation kind and sample count. Persist 90 days and show cards plus a daily trend.
2. **Diagnosis:** add blocking API duration and initial JS/CSS bytes. This directly tests whether route
   splitting and Chakra changes improve real loads.
3. **Attribution:** only if INP or LCP is poor, switch to the attribution build and retain a small,
   privacy-safe allowlist of breakdown fields. LCP can be decomposed into TTFB, resource-load delay,
   resource-load duration and element-render delay
   ([LCP optimization guide](https://web.dev/articles/optimize-lcp)).

This order establishes a trustworthy before/after baseline before the next bundle optimization while
keeping the telemetry itself small enough not to become a new performance problem.
