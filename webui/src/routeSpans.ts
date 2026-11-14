// Times each router navigation from its start to the page calling useReady(). A span that is
// abandoned, redirected away, or started by a rule below records nothing.
import type { DataRouter } from "react-router"
import { routeName } from "./routeNames"
import { queueSample, type WebPerfSample } from "./webPerf"

type Trigger = NonNullable<WebPerfSample["trigger"]>
export type Outcome = NonNullable<WebPerfSample["outcome"]>
export type DataSource = NonNullable<WebPerfSample["data_source"]>
type NavigationType = NonNullable<WebPerfSample["navigation_type"]>

// A pointer or key press this recent is the cause of a navigation.
const INPUT_WINDOW_MS = 1000

type Span = {
  start: number
  trigger: Trigger
  // The first span of a page load is stored as app_ready, from navigation start.
  hard: boolean
  // Spans that finish but report nothing: catch-all redirect, first span after sign-in.
  skip: boolean
  route?: string
  done: boolean
}

let span: Span | undefined
let hardPending = true
let skipFirst = false
let catchAll = false
let hardDropped = false
let bfcache = false
let lastInput: { trigger: Trigger; at: number } | undefined

/** Test-only: module state outlives a test. */
export function resetRouteSpans(): void {
  span = undefined
  hardPending = true
  skipFirst = false
  catchAll = false
  hardDropped = false
  bfcache = false
  lastInput = undefined
  apiRequests = []
}

/** The sign-in or setup screen showed: this load has no app_ready and the app's first span after it reports nothing. */
export function noteAuthScreen(): void {
  hardDropped = true
  skipFirst = true
}

/** A sign-out after the app ran is not a hard load, so the next first span reports nothing. */
export function noteSignedOut(): void {
  skipFirst = true
}

/** The catch-all route redirected an unknown path: that navigation reports nothing. */
export function noteCatchAll(): void {
  catchAll = true
}

/** True once the sign-in or setup screen showed on this load; the hard-load metrics are dropped. */
export function hardLoadDropped(): boolean {
  return hardDropped
}

/** `navigation_type` of this load's hard-load samples. */
export function hardLoadNavigationType(): NavigationType | undefined {
  if (bfcache) return "back-forward-cache"
  const type = (
    performance.getEntriesByType?.("navigation")[0] as PerformanceNavigationTiming | undefined
  )?.type
  if (type === "reload" || type === "navigate") return type
  if (type === "back_forward") return "back-forward"
  return type === "prerender" ? "navigate" : undefined
}

// Web API requests of the current span, from apiFetch. Only times, never the URL.
type ApiRequest = { start: number; end?: number }
let apiRequests: ApiRequest[] = []

/** apiFetch calls this when a request starts. Call the returned function when it settles. */
export function noteApiRequest(): () => void {
  const request: ApiRequest = { start: performance.now() }
  apiRequests.push(request)
  return () => {
    request.end = performance.now()
  }
}

/** The longest request that overlapped the span, in ms. Undefined when there was none. */
function slowestApiRequest(spanStart: number, spanEnd: number): number | undefined {
  let slowest: number | undefined
  for (const r of apiRequests) {
    const end = r.end ?? spanEnd
    if (end < spanStart || r.start > spanEnd) continue
    slowest = Math.max(slowest ?? 0, end - r.start)
  }
  return slowest
}

/** Time to first byte of the page load. Undefined when the browser reports none. */
function ttfb(): number | undefined {
  const nav = performance.getEntriesByType?.("navigation")[0] as
    | PerformanceNavigationTiming
    | undefined
  return nav && nav.responseStart > 0 ? nav.responseStart : undefined
}

/** Transferred bytes of this origin's scripts and stylesheets. A sum of zero means cached or unknown, so it is left out. */
function transferredBytes(): Pick<WebPerfSample, "js_bytes" | "css_bytes"> {
  let js = 0
  let css = 0
  for (const e of performance.getEntriesByType?.("resource") ?? []) {
    const entry = e as PerformanceResourceTiming
    if (!["script", "link", "css"].includes(entry.initiatorType)) continue
    let url: URL
    try {
      url = new URL(entry.name)
    } catch {
      continue
    }
    if (url.origin !== location.origin) continue
    // A link entry is a stylesheet or a modulepreload, so the file extension decides.
    if (url.pathname.endsWith(".css")) css += entry.transferSize
    else if (url.pathname.endsWith(".js")) js += entry.transferSize
  }
  return { ...(js > 0 && { js_bytes: js }), ...(css > 0 && { css_bytes: css }) }
}

function recentInput(): Trigger | undefined {
  return lastInput && performance.now() - lastInput.at < INPUT_WINDOW_MS
    ? lastInput.trigger
    : undefined
}

// `final` is false while the router still runs its first loaders: a catch-all redirect may be
// pending, and its flag belongs to the commit that follows.
function begin(
  trigger: Trigger,
  route: string | undefined,
  redirect: boolean,
  final: boolean,
): void {
  if (redirect && span && !span.done) {
    // A known redirect is one span for the final route.
    span.route = route
  } else {
    const hard = hardPending
    hardPending = false
    performance.mark("route_start")
    apiRequests = apiRequests.filter((r) => r.end === undefined)
    span = {
      start: hard ? 0 : performance.now(),
      trigger,
      hard,
      skip: skipFirst,
      route,
      done: false,
    }
    skipFirst = false
  }
  if (catchAll && final) {
    span.skip = true
    catchAll = false
  }
}

/** Called by useReady: the first call for a span wins. */
export function markReady(outcome: Outcome, dataSource: DataSource): void {
  const s = span
  if (!s || s.done) return
  s.done = true
  const end = performance.mark("route_ready").startTime
  if (s.skip || !s.route) return
  const latency = slowestApiRequest(s.start, end)
  const common = {
    route: s.route,
    outcome,
    data_source: dataSource,
    ...(latency !== undefined && { api_latency: latency }),
  }
  apiRequests = apiRequests.filter((r) => r.end === undefined)
  if (s.hard) {
    const first = ttfb()
    queueSample({
      ...common,
      metric: "app_ready",
      value: end,
      tab: "hard",
      navigation_type: hardLoadNavigationType(),
      ...(first !== undefined && { ttfb: first }),
      ...transferredBytes(),
    })
  } else {
    queueSample({
      ...common,
      metric: "route_ready",
      value: end - s.start,
      tab: "route",
      trigger: s.trigger,
    })
  }
}

/** Starts a span per router navigation. Returns the cleanup. */
export function trackRouter(router: Pick<DataRouter, "state" | "subscribe">): () => void {
  let key = router.state.location.key
  const routeOf = (state: DataRouter["state"]) => routeName(state.matches.at(-1)?.route.path)

  // The router is created before this runs, so the page's own load is the first span.
  begin("programmatic", routeOf(router.state), false, router.state.initialized)

  const unsubscribe = router.subscribe((state) => {
    if (state.navigation.state !== "idle" || state.location.key === key) return
    key = state.location.key
    const trigger = state.historyAction === "POP" ? "history" : (recentInput() ?? "programmatic")
    begin(trigger, routeOf(state), state.historyAction === "REPLACE", true)
  })

  const onInput = (trigger: Trigger) => () => {
    lastInput = { trigger, at: performance.now() }
  }
  const onPointer = onInput("click")
  const onKey = onInput("keyboard")
  const abandon = () => {
    if (span && !span.done) span.done = true
  }
  const onHide = () => {
    if (document.visibilityState === "hidden") abandon()
  }
  const onPageShow = (e: PageTransitionEvent) => {
    if (!e.persisted) return
    bfcache = true
    abandon()
  }
  document.addEventListener("pointerdown", onPointer, true)
  document.addEventListener("keydown", onKey, true)
  document.addEventListener("visibilitychange", onHide)
  window.addEventListener("pageshow", onPageShow)
  return () => {
    unsubscribe()
    abandon()
    document.removeEventListener("pointerdown", onPointer, true)
    document.removeEventListener("keydown", onKey, true)
    document.removeEventListener("visibilitychange", onHide)
    window.removeEventListener("pageshow", onPageShow)
  }
}
