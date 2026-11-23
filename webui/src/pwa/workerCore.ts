// Fetch rules of the service worker, kept free of `self` so vitest can drive
// them with a fake `caches` and `fetch`. The worker entry only wires events.

export const SHELL_TIMEOUT_MS = 1500

export interface FetchEventLike {
  request: Request
  respondWith(response: Promise<Response>): void
  waitUntil(work: Promise<unknown>): void
}

export interface WorkerDeps {
  caches: Pick<CacheStorage, "open">
  fetch: (input: string, init?: RequestInit) => Promise<Response>
  cacheName: string
  shellTimeoutMs?: number
}

// Every cache the worker opens starts with this, so Force refresh can delete
// exactly the app shell caches and nothing else's.
export const CACHE_PREFIX = "gx-shell-"

export interface LifecycleDeps {
  caches: Pick<CacheStorage, "open" | "keys" | "delete">
  fetch: (input: string, init?: RequestInit) => Promise<Response>
  origin: string
  buildId: string
  // Site-relative URLs to precache; "/" is the shell.
  urls: string[]
}

export function cacheNameFor(buildId: string): string {
  return CACHE_PREFIX + buildId
}

// Fetch everything before opening the cache, so one failed asset leaves no
// half-set cache behind; the shell goes in last, after the assets it loads.
// Each body is read as its fetch returns: a service worker in Chrome serves
// bodies left unread only a few at a time, which made the install take
// seconds (docs/research/test-service-worker-activation.md).
export async function precache(deps: LifecycleDeps): Promise<void> {
  const fetched = await Promise.all(
    deps.urls.map(async (path) => {
      const url = deps.origin + path
      const fetchedResponse = await deps.fetch(url, { cache: "reload" })
      if (fetchedResponse.status !== 200) {
        throw new Error(`precache ${url}: status ${fetchedResponse.status}`)
      }
      const bytes = await fetchedResponse.arrayBuffer()
      const response = new Response(bytes, {
        status: fetchedResponse.status,
        statusText: fetchedResponse.statusText,
        headers: fetchedResponse.headers,
      })
      return { url, response }
    }),
  )
  const cache = await deps.caches.open(cacheNameFor(deps.buildId))
  const shellURL = `${deps.origin}/`
  const ordered = [
    ...fetched.filter((f) => f.url !== shellURL),
    ...fetched.filter((f) => f.url === shellURL),
  ]
  for (const { url, response } of ordered) await cache.put(url, response)
}

export async function pruneOldCaches(
  deps: Pick<LifecycleDeps, "caches" | "buildId">,
): Promise<void> {
  const current = cacheNameFor(deps.buildId)
  const names = await deps.caches.keys()
  await Promise.all(
    names
      .filter((n) => n.startsWith(CACHE_PREFIX) && n !== current)
      .map((n) => deps.caches.delete(n)),
  )
}

const STALE_WHILE_REVALIDATE =
  /^\/(manifest\.webmanifest|pwa-icon[^/]*\.png|apple-touch-icon\.png|favicon\.png)$/
const ASSET_REF = /(?:src|href)="(\/assets\/[^"]+)"/g

// A refreshed index.html is stored only when every asset it references is
// already cached, so an offline open never loads a shell whose scripts are
// missing; otherwise the old copy stays.
async function refreshShell(cache: Cache, shellURL: string, origin: string, response: Response) {
  const html = await response.text()
  for (const match of html.matchAll(ASSET_REF)) {
    if (!(await cache.match(origin + match[1]))) return
  }
  await cache.put(shellURL, new Response(html, { headers: response.headers }))
}

export function createFetchHandler(deps: WorkerDeps) {
  const timeoutMs = deps.shellTimeoutMs ?? SHELL_TIMEOUT_MS
  const open = () => deps.caches.open(deps.cacheName)

  async function shell(event: FetchEventLike, shellURL: string, origin: string): Promise<Response> {
    const cache = await open()
    const network = deps.fetch(shellURL, { cache: "no-cache" })
    // The refresh is a side effect: it never delays or fails the answer. A slow
    // answer still refreshes the copy, so keep the worker alive for it.
    event.waitUntil(
      network
        .then((r) =>
          r.status === 200 ? refreshShell(cache, shellURL, origin, r.clone()) : undefined,
        )
        .catch(() => undefined),
    )
    const timeout = new Promise<undefined>((resolve) =>
      setTimeout(() => resolve(undefined), timeoutMs),
    )
    const first = await Promise.race([network.catch(() => undefined), timeout])
    if (first?.status === 200) return first
    // No copy to fall back on: hand back whatever the network gives.
    return (await cache.match(shellURL)) ?? network
  }

  async function cacheFirst(request: Request): Promise<Response> {
    const cache = await open()
    const cached = await cache.match(request.url)
    if (cached) return cached
    const response = await deps.fetch(request.url)
    if (response.status === 200) await cache.put(request.url, response.clone())
    return response
  }

  async function staleWhileRevalidate(event: FetchEventLike): Promise<Response> {
    const url = event.request.url
    const cache = await open()
    const cached = await cache.match(url)
    const refresh = deps.fetch(url).then(async (r) => {
      if (r.status === 200) await cache.put(url, r.clone())
      return r
    })
    if (!cached) return refresh
    event.waitUntil(refresh.catch(() => undefined))
    return cached
  }

  return function onFetch(event: FetchEventLike): void {
    const { request } = event
    if (request.method !== "GET") return
    const url = new URL(request.url)
    const path = url.pathname
    if (path.startsWith("/v1/")) return
    if (request.mode === "navigate" || path === "/" || path === "/index.html") {
      event.respondWith(shell(event, `${url.origin}/`, url.origin))
    } else if (path.startsWith("/assets/")) {
      event.respondWith(cacheFirst(request))
    } else if (STALE_WHILE_REVALIDATE.test(path)) {
      event.respondWith(staleWhileRevalidate(event))
    }
  }
}

// The page listens for this message and answers on the port it was sent with.
export const TAP_MESSAGE = "gx-notification-tap"
// How long a tapped window gets to answer before it counts as having no listener.
export const TAP_ACK_MS = 500
// Route of 06 (server/webpush/routes.go pathRenew).
export const PUSH_RENEW_PATH = "/v1/web/push/renew"

interface PushPayload {
  title?: string
  body?: string
  path?: string
  badge?: number
  tag?: string
}

export interface PushEventLike {
  data: { json(): unknown } | null
  waitUntil(work: Promise<unknown>): void
}

export interface PushDeps {
  showNotification: (title: string, options: NotificationOptions) => Promise<void>
  setAppBadge?: (count: number) => Promise<void>
  clearAppBadge?: () => Promise<void>
}

export function createPushHandler(deps: PushDeps) {
  async function show(payload: PushPayload) {
    await deps.showNotification(payload.title || "gx", {
      body: payload.body,
      tag: payload.tag,
      data: { path: payload.path },
    })
    if (typeof payload.badge !== "number") return
    try {
      if (payload.badge > 0) await deps.setAppBadge?.(payload.badge)
      else await deps.clearAppBadge?.()
    } catch {
      // A badge error never undoes the notification.
    }
  }

  return function onPush(event: PushEventLike): void {
    let payload: PushPayload = {}
    try {
      const parsed = event.data?.json()
      if (parsed && typeof parsed === "object") payload = parsed as PushPayload
    } catch {
      // An unreadable payload still gets a notification: iOS requires one.
    }
    event.waitUntil(show(payload))
  }
}

export interface WindowClientLike {
  focus(): Promise<unknown>
  navigate(url: string): Promise<unknown>
  postMessage(message: unknown, transfer: Transferable[]): void
}

export interface TapEventLike {
  notification: { close(): void; data?: { path?: string } | null }
  waitUntil(work: Promise<unknown>): void
}

export interface TapDeps {
  origin: string
  matchWindows: () => Promise<WindowClientLike[]>
  openWindow: (url: string) => Promise<unknown>
  ackMs?: number
}

export function createTapHandler(deps: TapDeps) {
  const ackMs = deps.ackMs ?? TAP_ACK_MS

  async function answered(client: WindowClientLike, path: string): Promise<boolean> {
    const channel = new MessageChannel()
    let timer: ReturnType<typeof setTimeout> | undefined
    const ack = new Promise<boolean>((resolve) => {
      channel.port1.onmessage = () => resolve(true)
      timer = setTimeout(() => resolve(false), ackMs)
    })
    client.postMessage({ type: TAP_MESSAGE, path }, [channel.port2])
    const ok = await ack
    clearTimeout(timer)
    channel.port1.close()
    return ok
  }

  async function open(path: string) {
    const url = `${deps.origin}/#${path}`
    const [client] = await deps.matchWindows()
    if (!client) {
      await deps.openWindow(url)
      return
    }
    await client.focus()
    // No answer means the page has no listener yet: only a reload gets the path to it.
    if (!(await answered(client, path))) await client.navigate(url)
  }

  return function onTap(event: TapEventLike): void {
    event.notification.close()
    event.waitUntil(open(event.notification.data?.path ?? "/").catch(() => undefined))
  }
}

export interface SubscriptionLike {
  endpoint: string
  options?: { applicationServerKey?: ArrayBuffer | null }
  toJSON(): { endpoint?: string; keys?: { p256dh?: string; auth?: string } }
}

export interface RenewEventLike {
  oldSubscription?: { endpoint: string; options?: SubscriptionLike["options"] } | null
  newSubscription?: SubscriptionLike | null
  waitUntil(work: Promise<unknown>): void
}

export interface RenewDeps {
  subscribe: (key: ArrayBuffer | undefined) => Promise<SubscriptionLike>
  fetch: (input: string, init?: RequestInit) => Promise<Response>
}

function base64url(buf: ArrayBuffer | null | undefined): string {
  if (!buf) return ""
  let s = ""
  for (const b of new Uint8Array(buf)) s += String.fromCharCode(b)
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

export function createRenewHandler(deps: RenewDeps) {
  async function renew(event: RenewEventLike) {
    const old = event.oldSubscription
    const key = old?.options?.applicationServerKey ?? undefined
    const sub = event.newSubscription ?? (await deps.subscribe(key))
    const json = sub.toJSON()
    await deps.fetch(PUSH_RENEW_PATH, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        endpoint: json.endpoint ?? sub.endpoint,
        p256dh: json.keys?.p256dh ?? "",
        auth: json.keys?.auth ?? "",
        vapid_key: base64url(key ?? sub.options?.applicationServerKey),
        old_endpoint: old?.endpoint ?? "",
      }),
    })
  }

  return function onRenew(event: RenewEventLike): void {
    // A refusal or a network error leaves the stored subscription as it was.
    event.waitUntil(renew(event).catch(() => undefined))
  }
}
