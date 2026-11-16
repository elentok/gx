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

// Web API requests of the current span, from apiFetch. Only times, never the URL.
type ApiRequest = { start: number; end?: number }

/** Maps the Navigation Timing and web-vitals spellings to a sample's `navigation_type`. */
export function toNavigationType(type: string | undefined): NavigationType | undefined {
  switch (type) {
    case "navigate":
    case "reload":
    case "back-forward":
    case "back-forward-cache":
      return type
    case "back_forward":
      return "back-forward"
    case "prerender":
      return "navigate"
    default:
      return undefined
  }
}

// All span state lives here, and every change is a named transition.
class SpanTracker {
  span: Span | undefined
  hardPending = true
  skipFirst = false
  catchAll = false
  hardDropped = false
  bfcache = false
  lastInput: { trigger: Trigger; at: number } | undefined
  apiRequests: ApiRequest[] = []

  reset(): void {
    Object.assign(this, new SpanTracker())
  }

  /** The sign-in or setup screen showed: no app_ready, and the first span after it reports nothing. */
  authScreenShown(): void {
    this.hardDropped = true
    this.skipFirst = true
  }

  /** A sign-out after the app ran is not a hard load. */
  signedOut(): void {
    this.skipFirst = true
  }

  /** The catch-all route redirected: the next span to settle reports nothing. */
  catchAllRedirected(): void {
    this.catchAll = true
  }

  /** The page's own load. `settled` is false while the router still runs its first loaders. */
  loadStarted(route: string | undefined, settled: boolean): void {
    this.open("programmatic", route)
    // A catch-all redirect may still be pending, and its flag belongs to the commit that follows.
    if (settled) this.applyCatchAll()
  }

  /** A router navigation that is its own span. */
  navigated(trigger: Trigger, route: string | undefined): void {
    this.open(trigger, route)
    this.applyCatchAll()
  }

  /** A known redirect is one span for the final route. */
  redirected(trigger: Trigger, route: string | undefined): void {
    if (this.span && !this.span.done) this.span.route = route
    else this.open(trigger, route)
    this.applyCatchAll()
  }

  /** The tab came back from the back/forward cache: this is no longer a fresh load. */
  restoredFromBfcache(): void {
    this.bfcache = true
    this.abandon()
  }

  abandon(): void {
    if (this.span && !this.span.done) this.span.done = true
  }

  inputSeen(trigger: Trigger): void {
    this.lastInput = { trigger, at: performance.now() }
  }

  recentInput(): Trigger | undefined {
    const input = this.lastInput
    return input && performance.now() - input.at < INPUT_WINDOW_MS ? input.trigger : undefined
  }

  /** Request times are kept only while a span is open: between spans nothing reads them. */
  requestStarted(request: ApiRequest): void {
    if (this.span && !this.span.done) this.apiRequests.push(request)
  }

  /** Drops finished requests; the ones still in flight carry over to the next span. */
  keepInFlightRequests(): void {
    this.apiRequests = this.apiRequests.filter((r) => r.end === undefined)
  }

  private open(trigger: Trigger, route: string | undefined): void {
    const hard = this.hardPending
    this.hardPending = false
    performance.mark("route_start")
    this.keepInFlightRequests()
    this.span = {
      start: hard ? 0 : performance.now(),
      trigger,
      hard,
      skip: this.skipFirst,
      route,
      done: false,
    }
    this.skipFirst = false
  }

  // Must run after open() or the redirect fold, so the flag lands on the span it belongs to.
  private applyCatchAll(): void {
    if (!this.catchAll || !this.span) return
    this.span.skip = true
    this.catchAll = false
  }
}

const tracker = new SpanTracker()

/** Test-only: module state outlives a test. */
export function resetRouteSpans(): void {
  tracker.reset()
}

export const noteAuthScreen = (): void => tracker.authScreenShown()
export const noteSignedOut = (): void => tracker.signedOut()
export const noteCatchAll = (): void => tracker.catchAllRedirected()

/** True once the sign-in or setup screen showed on this load; the hard-load metrics are dropped. */
export function hardLoadDropped(): boolean {
  return tracker.hardDropped
}

/** `navigation_type` of this load's hard-load samples. */
export function hardLoadNavigationType(): NavigationType | undefined {
  if (tracker.bfcache) return "back-forward-cache"
  return toNavigationType(
    (performance.getEntriesByType?.("navigation")[0] as PerformanceNavigationTiming | undefined)
      ?.type,
  )
}

/** apiFetch calls this when a request starts. Call the returned function when it settles. */
export function noteApiRequest(): () => void {
  const request: ApiRequest = { start: performance.now() }
  tracker.requestStarted(request)
  return () => {
    request.end = performance.now()
  }
}

/** The longest request that overlapped the span, in ms. Undefined when there was none. */
function slowestApiRequest(spanStart: number, spanEnd: number): number | undefined {
  let slowest: number | undefined
  for (const r of tracker.apiRequests) {
    const end = r.end ?? spanEnd
    if (end < spanStart || r.start > spanEnd) continue
    // A request carried over from an earlier span only counts from this span's start.
    slowest = Math.max(slowest ?? 0, end - Math.max(r.start, spanStart))
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

/** Called by useReady: the first call for a span wins. */
export function markReady(outcome: Outcome, dataSource: DataSource): void {
  const s = tracker.span
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
  tracker.keepInFlightRequests()
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

// Pages that redirect with Navigate/replace (see routes.tsx). Only a REPLACE away from one of
// these folds into the open span; any other REPLACE is its own navigation.
function isKnownRedirectSource(pathname: string): boolean {
  return (
    pathname === "/settings/agents" || pathname.startsWith("/d/") || pathname.startsWith("/triage")
  )
}

/** Starts a span per router navigation. Returns the cleanup. */
export function trackRouter(router: Pick<DataRouter, "state" | "subscribe">): () => void {
  let key = router.state.location.key
  let previousPath = router.state.location.pathname
  const routeOf = (state: DataRouter["state"]) => routeName(state.matches.at(-1)?.route.path)

  // The router is created before this runs, so the page's own load is the first span.
  tracker.loadStarted(routeOf(router.state), router.state.initialized)

  const unsubscribe = router.subscribe((state) => {
    if (state.navigation.state !== "idle" || state.location.key === key) return
    key = state.location.key
    const trigger =
      state.historyAction === "POP" ? "history" : (tracker.recentInput() ?? "programmatic")
    const redirect = state.historyAction === "REPLACE" && isKnownRedirectSource(previousPath)
    previousPath = state.location.pathname
    if (redirect) tracker.redirected(trigger, routeOf(state))
    else tracker.navigated(trigger, routeOf(state))
  })

  const onPointer = () => tracker.inputSeen("click")
  const onKey = () => tracker.inputSeen("keyboard")
  const abandon = () => tracker.abandon()
  // A tab switch only fires visibilitychange, and the load goes on: only leaving the page abandons.
  const onHide = () => abandon()
  const onPageShow = (e: PageTransitionEvent) => {
    if (e.persisted) tracker.restoredFromBfcache()
  }
  document.addEventListener("pointerdown", onPointer, true)
  document.addEventListener("keydown", onKey, true)
  window.addEventListener("pagehide", onHide)
  window.addEventListener("pageshow", onPageShow)
  return () => {
    unsubscribe()
    abandon()
    document.removeEventListener("pointerdown", onPointer, true)
    document.removeEventListener("keydown", onKey, true)
    window.removeEventListener("pagehide", onHide)
    window.removeEventListener("pageshow", onPageShow)
  }
}
