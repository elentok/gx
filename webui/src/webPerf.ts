// Holds performance samples and sends them to the server. It measures nothing itself.
// Plain fetch, not apiFetch: a 401 from telemetry must never sign the page out.
const ROUTE = "/v1/web-performance"
const SCHEMA_VERSION = 1
const FLUSH_AT = 20
const IDLE_MS = 10_000

// Field and enum names match server/web/webperf.go; the server drops a sample it finds invalid.
export type WebPerfSample = {
  id: string
  metric: "lcp" | "inp" | "cls" | "fcp" | "app_ready" | "route_ready"
  value: number
  ts: number
  route?: string
  env?: "prod" | "dev"
  navigation_type?: "navigate" | "reload" | "back-forward" | "back-forward-cache"
  tab?: "hard" | "route"
  outcome?: "ready" | "error" | "empty"
  trigger?: "click" | "keyboard" | "history" | "programmatic"
  data_source?: "cache" | "network"
  ttfb?: number
  api_latency?: number
  js_bytes?: number
  css_bytes?: number
  input_delay?: number
  processing?: number
  presentation_delay?: number
  interaction_type?: "pointer" | "keyboard"
  action?: string
  worst_chunk?: string
  worst_frame?: number
  lcp_load_delay?: number
  lcp_load_duration?: number
  lcp_render_delay?: number
}

export type WebPerfInput = Omit<WebPerfSample, "id" | "ts">

// Per page load, in memory only: a reload is a new session.
const session = randomId()
let nextId = 0
let queue: WebPerfSample[] = []
let enabled = false
let idleTimer: ReturnType<typeof setTimeout> | undefined

function randomId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("")
}

// The entry chunk hash (`assets/index-<hash>.js`); the Vite dev server has none.
function buildId(): string {
  const src = document.querySelector<HTMLScriptElement>("script[type=module][src]")?.src ?? ""
  return /\/assets\/[^/-]+-([A-Za-z0-9_-]+)\.js(?:$|\?)/.exec(src)?.[1] ?? "dev"
}

/** The auth gate calls this once it has passed; until then samples are discarded. */
export function enableQueue(): void {
  enabled = true
  listen()
  for (const fn of whenEnabled.splice(0)) fn()
}

const whenEnabled: Array<() => void> = []

/** Runs fn now if the queue is on, else when it turns on. */
export function afterQueueEnabled(fn: () => void): void {
  if (enabled) fn()
  else whenEnabled.push(fn)
}

export function queueSample(input: WebPerfInput): void {
  if (!enabled) return
  const build = buildId()
  queue.push({
    ...input,
    ...(build === "dev" && { env: "dev" as const }),
    id: String(nextId++),
    ts: Date.now(),
  })
  clearTimeout(idleTimer)
  if (queue.length >= FLUSH_AT) {
    flush(false)
    return
  }
  idleTimer = setTimeout(() => flush(false), IDLE_MS)
}

/** Clears the queue and the idle timer. Called by signOut; never waits and never throws. */
export function dropQueue(): void {
  clearTimeout(idleTimer)
  queue = []
}

function post(body: string): void {
  fetch(ROUTE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    keepalive: true,
  }).catch(() => {})
}

function flush(hidden: boolean): void {
  clearTimeout(idleTimer)
  if (queue.length === 0) return
  const body = JSON.stringify({
    v: SCHEMA_VERSION,
    session,
    build_id: buildId(),
    web_version: __WEB_VERSION__,
    samples: queue,
  })
  queue = []
  // A failed send drops the batch: no retry, no UI. The response is never read, so a refusal
  // (200 with refused: true) and a non-2xx answer need no handling.
  try {
    if (hidden && navigator.sendBeacon?.(ROUTE, new Blob([body], { type: "application/json" }))) {
      return
    }
  } catch {
    // Fall through to fetch.
  }
  try {
    post(body)
  } catch {
    // fetch threw synchronously.
  }
}

// Registered on the first enableQueue, not at import: node-environment tests import apiFetch.
let listening = false
function listen(): void {
  if (listening) return
  listening = true
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") flush(true)
  })
}
