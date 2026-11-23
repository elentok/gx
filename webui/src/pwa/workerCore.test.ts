// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  CACHE_PREFIX,
  cacheNameFor,
  createFetchHandler,
  createPushHandler,
  createRenewHandler,
  createTapHandler,
  type FetchEventLike,
  precache,
  PUSH_RENEW_PATH,
  type PushDeps,
  pruneOldCaches,
  type RenewEventLike,
  TAP_ACK_MS,
  TAP_MESSAGE,
} from "./workerCore"

const ORIGIN = "https://gx.test"
const SHELL_URL = `${ORIGIN}/`
const HTML = `<html><script src="/assets/app-1.js"></script><link href="/assets/app-1.css"></html>`

function fakeCaches(failPut = false) {
  const store = new Map<string, Response>()
  const cache = {
    async match(req: string | Request) {
      const hit = store.get(typeof req === "string" ? req : req.url)
      return hit?.clone()
    },
    async put(req: string | Request, res: Response) {
      if (failPut) throw new Error("quota exceeded")
      store.set(typeof req === "string" ? req : req.url, res)
    },
  }
  return { store, caches: { open: async () => cache as unknown as Cache } }
}

type Fetch = (input: string, init?: RequestInit) => Promise<Response>

function setup(fetchImpl: Fetch, opts: { failPut?: boolean } = {}) {
  const { store, caches } = fakeCaches(opts.failPut)
  const fetchSpy = vi.fn(fetchImpl)
  const onFetch = createFetchHandler({ caches, fetch: fetchSpy, cacheName: "t" })
  const send = (path: string, init: { method?: string; mode?: RequestMode } = {}) => {
    const request = new Request(ORIGIN + path, { method: init.method ?? "GET" })
    if (init.mode) Object.defineProperty(request, "mode", { value: init.mode })
    const waits: Promise<unknown>[] = []
    let answer: Promise<Response> | undefined
    const event: FetchEventLike = {
      request,
      respondWith: (p) => {
        answer = p
      },
      waitUntil: (p) => {
        waits.push(p)
      },
    }
    onFetch(event)
    return { answer, settle: () => Promise.all(waits) }
  }
  return { store, fetchSpy, send }
}

const html = (body = HTML, status = 200) => new Response(body, { status })

async function seedAssets(store: Map<string, Response>) {
  store.set(`${ORIGIN}/assets/app-1.js`, new Response("js"))
  store.set(`${ORIGIN}/assets/app-1.css`, new Response("css"))
}

describe("service worker fetch rules", () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it("answers a fast 200 from the network and refreshes the copy", async () => {
    const { store, fetchSpy, send } = setup(async () => html())
    await seedAssets(store)
    store.set(SHELL_URL, html("old"))
    const { answer, settle } = send("/", { mode: "navigate" })
    expect(await (await answer)!.text()).toBe(HTML)
    await settle()
    expect(await store.get(SHELL_URL)!.text()).toBe(HTML)
    expect(fetchSpy).toHaveBeenCalledWith(SHELL_URL, { cache: "no-cache" })
  })

  it("still answers a 200 when the cache write rejects", async () => {
    const { store, send } = setup(async () => html(), { failPut: true })
    await seedAssets(store)
    const { answer, settle } = send("/", { mode: "navigate" })
    expect(await (await answer)!.text()).toBe(HTML)
    await settle()
  })

  it("answers from the cache after 1.5s, then refreshes when the slow request lands", async () => {
    let land!: (r: Response) => void
    const { store, send } = setup(() => new Promise<Response>((r) => (land = r)))
    await seedAssets(store)
    store.set(SHELL_URL, html("old"))
    const { answer, settle } = send("/")
    await vi.advanceTimersByTimeAsync(1500)
    expect(await (await answer)!.text()).toBe("old")
    land(html())
    await settle()
    expect(await store.get(SHELL_URL)!.text()).toBe(HTML)
  })

  it("answers from the cache at once on a network error and refreshes nothing", async () => {
    const { store, send } = setup(async () => {
      throw new TypeError("refused")
    })
    store.set(SHELL_URL, html("old"))
    const { answer } = send("/")
    expect(await (await answer)!.text()).toBe("old")
    expect(await store.get(SHELL_URL)!.text()).toBe("old")
  })

  it("answers from the cache at once on a 502 and refreshes nothing", async () => {
    const { store, send } = setup(async () => html("bad gateway", 502))
    store.set(SHELL_URL, html("old"))
    const { answer, settle } = send("/")
    expect(await (await answer)!.text()).toBe("old")
    await settle()
    expect(await store.get(SHELL_URL)!.text()).toBe("old")
  })

  it("keeps the old copy when the new index references uncached assets", async () => {
    const { store, send } = setup(async () => html())
    store.set(SHELL_URL, html("old"))
    const { answer, settle } = send("/")
    expect(await (await answer)!.text()).toBe(HTML)
    await settle()
    expect(await store.get(SHELL_URL)!.text()).toBe("old")
  })

  it("leaves /v1 requests and non-GET requests alone", async () => {
    const { store, fetchSpy, send } = setup(async () => html())
    for (const path of ["/v1/snapshot", "/v1/events", "/v1/web/auth"]) {
      expect(send(path).answer).toBeUndefined()
    }
    expect(send("/", { method: "POST" }).answer).toBeUndefined()
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(store.size).toBe(0)
  })

  it("serves assets cache-first", async () => {
    const { store, fetchSpy, send } = setup(async () => new Response("net"))
    const first = send("/assets/app-2.js")
    expect(await (await first.answer)!.text()).toBe("net")
    expect(store.has(`${ORIGIN}/assets/app-2.js`)).toBe(true)
    const second = send("/assets/app-2.js")
    expect(await (await second.answer)!.text()).toBe("net")
    expect(fetchSpy).toHaveBeenCalledTimes(1)
  })

  it("serves the manifest stale-while-revalidate", async () => {
    const { store, send } = setup(async () => new Response("new"))
    store.set(`${ORIGIN}/manifest.webmanifest`, new Response("old"))
    const { answer, settle } = send("/manifest.webmanifest")
    expect(await (await answer)!.text()).toBe("old")
    await settle()
    expect(await store.get(`${ORIGIN}/manifest.webmanifest`)!.text()).toBe("new")
  })

  it("gives no cached answer when there is no copy and the server is down", async () => {
    const { store, send } = setup(async () => {
      throw new TypeError("refused")
    })
    const { answer } = send("/")
    await expect(answer).rejects.toThrow("refused")
    expect(store.size).toBe(0)
  })
})

function namedCaches() {
  const all = new Map<string, Map<string, Response>>()
  const caches = {
    async open(name: string) {
      if (!all.has(name)) all.set(name, new Map())
      const store = all.get(name)!
      return {
        async match(req: string) {
          return store.get(req)?.clone()
        },
        async put(req: string, res: Response) {
          store.set(req, res)
        },
      } as unknown as Cache
    },
    async keys() {
      return [...all.keys()]
    },
    async delete(name: string) {
      return all.delete(name)
    },
  }
  return { all, caches }
}

describe("service worker lifecycle", () => {
  const urls = [
    "/",
    "/assets/app-1.js",
    "/assets/app-1.css",
    "/manifest.webmanifest",
    "/pwa-icon-192.png",
  ]
  const body: Fetch = async (url) => new Response(url.endsWith("/") ? HTML : "x")
  const deps = (
    caches: ReturnType<typeof namedCaches>["caches"],
    fetch: Fetch,
    buildId = "b1",
  ) => ({
    caches,
    fetch,
    origin: ORIGIN,
    buildId,
    urls,
  })

  it("precaches the shell and every listed asset under the build's cache name", async () => {
    const { all, caches } = namedCaches()
    await precache(deps(caches, body))
    expect(cacheNameFor("b1").startsWith(CACHE_PREFIX)).toBe(true)
    expect([...all.get(cacheNameFor("b1"))!.keys()].sort()).toEqual(
      urls.map((u) => ORIGIN + u).sort(),
    )
  })

  it("reads every response body before it opens the cache", async () => {
    const { caches } = namedCaches()
    const responses: Response[] = []
    const tracking: Fetch = async (url) => {
      const res = await body(url)
      responses.push(res)
      return res
    }
    let unreadAtOpen = -1
    const open = caches.open.bind(caches)
    caches.open = async (name: string) => {
      unreadAtOpen = responses.filter((r) => !r.bodyUsed).length
      return open(name)
    }
    await precache(deps(caches, tracking))
    expect(responses).toHaveLength(urls.length)
    expect(unreadAtOpen).toBe(0)
  })

  it("fails the install and leaves no cache when an asset fetch fails", async () => {
    const { all, caches } = namedCaches()
    const failing: Fetch = async (url) =>
      url.endsWith(".css") ? new Response("", { status: 404 }) : body(url)
    await expect(precache(deps(caches, failing))).rejects.toThrow("app-1.css")
    expect(all.size).toBe(0)
    const throwing: Fetch = async (url) => {
      if (url.endsWith(".js")) throw new TypeError("offline")
      return body(url)
    }
    await expect(precache(deps(caches, throwing))).rejects.toThrow("offline")
    expect(all.size).toBe(0)
  })

  it("stores the shell after the assets it references", async () => {
    const { all, caches } = namedCaches()
    await precache(deps(caches, body))
    expect([...all.get(cacheNameFor("b1"))!.keys()].at(-1)).toBe(SHELL_URL)
  })

  it("keeps the old build's cache when the new build's install fails", async () => {
    const { all, caches } = namedCaches()
    await precache(deps(caches, body, "b1"))
    await expect(
      precache(deps(caches, async () => new Response("", { status: 500 }), "b2")),
    ).rejects.toThrow()
    expect([...all.keys()]).toEqual([cacheNameFor("b1")])
  })

  it("prunes older builds' prefixed caches, keeps the current one, and never touches others", async () => {
    const { all, caches } = namedCaches()
    await precache(deps(caches, body, "b1"))
    await precache(deps(caches, body, "b2"))
    await caches.open("someone-elses")
    await pruneOldCaches({ caches, buildId: "b2" })
    expect([...all.keys()].sort()).toEqual([cacheNameFor("b2"), "someone-elses"].sort())
  })

  it("a changed build replaces the copy", async () => {
    const { all, caches } = namedCaches()
    await precache(
      deps(caches, async (u) => new Response(u.endsWith("/") ? "<html>one</html>" : "x"), "b1"),
    )
    await precache(
      deps(caches, async (u) => new Response(u.endsWith("/") ? "<html>two</html>" : "x"), "b2"),
    )
    await pruneOldCaches({ caches, buildId: "b2" })
    expect(all.has(cacheNameFor("b1"))).toBe(false)
    expect(await all.get(cacheNameFor("b2"))!.get(SHELL_URL)!.text()).toBe("<html>two</html>")
  })
})

function pushEvent(payload: unknown, raw?: string) {
  const work: Promise<unknown>[] = []
  return {
    event: {
      data:
        payload === undefined && raw === undefined
          ? null
          : { json: () => (raw !== undefined ? JSON.parse(raw) : payload) },
      waitUntil: (p: Promise<unknown>) => void work.push(p),
    },
    done: () => Promise.all(work),
  }
}

describe("service worker push", () => {
  const payload = { title: "T", body: "B", path: "/s/1", badge: 3, tag: "k" }
  const setup = (badge: Partial<PushDeps> = {}) => {
    const showNotification = vi.fn(async () => {})
    const setAppBadge = vi.fn(async () => {})
    const clearAppBadge = vi.fn(async () => {})
    const onPush = createPushHandler({ showNotification, setAppBadge, clearAppBadge, ...badge })
    return { onPush, showNotification, setAppBadge, clearAppBadge }
  }

  it("shows the payload's notification and sets the badge", async () => {
    const s = setup()
    const e = pushEvent(payload)
    s.onPush(e.event)
    await e.done()
    expect(s.showNotification).toHaveBeenCalledTimes(1)
    expect(s.showNotification).toHaveBeenCalledWith("T", {
      body: "B",
      tag: "k",
      data: { path: "/s/1" },
    })
    expect(s.setAppBadge).toHaveBeenCalledWith(3)
  })

  it("clears the badge at 0", async () => {
    const s = setup()
    const e = pushEvent({ ...payload, badge: 0 })
    s.onPush(e.event)
    await e.done()
    expect(s.clearAppBadge).toHaveBeenCalledTimes(1)
    expect(s.setAppBadge).not.toHaveBeenCalled()
  })

  it.each([
    ["missing", pushEvent(undefined)],
    ["unparsable", pushEvent(undefined, "{nope")],
  ])("still shows a notification for a %s payload", async (_name, e) => {
    const s = setup()
    s.onPush(e.event)
    await e.done()
    expect(s.showNotification).toHaveBeenCalledWith("gx", expect.anything())
  })

  it("a failing setAppBadge does not stop the notification or the handler", async () => {
    const s = setup({ setAppBadge: vi.fn(async () => Promise.reject(new Error("no"))) })
    const e = pushEvent(payload)
    s.onPush(e.event)
    await expect(e.done()).resolves.toBeDefined()
    expect(s.showNotification).toHaveBeenCalledTimes(1)
  })
})

describe("service worker notification tap", () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  const tapEvent = (path: string) => {
    const work: Promise<unknown>[] = []
    return {
      event: {
        notification: { close: vi.fn(), data: { path } },
        waitUntil: (p: Promise<unknown>) => void work.push(p),
      },
      done: () => Promise.all(work),
    }
  }
  const win = (answers: boolean) => ({
    focus: vi.fn(async () => {}),
    navigate: vi.fn(async () => {}),
    postMessage: vi.fn((_m: unknown, transfer: Transferable[]) => {
      if (answers) (transfer[0] as MessagePort).postMessage("ok")
    }),
  })
  const handler = (windows: ReturnType<typeof win>[]) => {
    const openWindow = vi.fn(async () => {})
    const onTap = createTapHandler({
      origin: ORIGIN,
      matchWindows: async () => windows,
      openWindow,
    })
    return { onTap, openWindow }
  }

  it("focuses an answering window and messages it without navigating or opening", async () => {
    const w = win(true)
    const { onTap, openWindow } = handler([w])
    const t = tapEvent("/s/1")
    onTap(t.event)
    await vi.advanceTimersByTimeAsync(0)
    await vi.waitFor(() => expect(w.postMessage).toHaveBeenCalled())
    await t.done()
    expect(t.event.notification.close).toHaveBeenCalled()
    expect(w.focus).toHaveBeenCalled()
    expect(w.postMessage.mock.calls[0][0]).toEqual({ type: TAP_MESSAGE, path: "/s/1" })
    expect(w.navigate).not.toHaveBeenCalled()
    expect(openWindow).not.toHaveBeenCalled()
  })

  it("navigates once after TAP_ACK_MS when the window never answers", async () => {
    const w = win(false)
    const { onTap } = handler([w])
    const t = tapEvent("/s/1")
    onTap(t.event)
    await vi.advanceTimersByTimeAsync(TAP_ACK_MS - 1)
    expect(w.navigate).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    await t.done()
    expect(w.navigate).toHaveBeenCalledTimes(1)
    expect(w.navigate).toHaveBeenCalledWith(`${ORIGIN}/#/s/1`)
  })

  it.each(["/s/abc", "/design/d1", "/tickets/gx/epic/01", "/tickets/gx/epic"])(
    "opens a window at /#%s when none is open",
    async (path) => {
      const { onTap, openWindow } = handler([])
      const t = tapEvent(path)
      onTap(t.event)
      await t.done()
      expect(openWindow).toHaveBeenCalledWith(`${ORIGIN}/#${path}`)
    },
  )
})

describe("service worker subscription renewal", () => {
  const sub = (endpoint: string) => ({
    endpoint,
    options: { applicationServerKey: new Uint8Array([251, 255]).buffer },
    toJSON: () => ({ endpoint, keys: { p256dh: "P", auth: "A" } }),
  })
  const run = async (fetchImpl: () => Promise<Response>, event: Partial<RenewEventLike>) => {
    const fetch = vi.fn(fetchImpl)
    const subscribe = vi.fn(async () => sub("https://push/new"))
    const work: Promise<unknown>[] = []
    createRenewHandler({ fetch, subscribe })({
      waitUntil: (p) => void work.push(p),
      ...event,
    })
    await Promise.all(work)
    return { fetch, subscribe }
  }

  it("posts the new subscription and the old endpoint to the renewal route", async () => {
    const { fetch, subscribe } = await run(async () => new Response("{}"), {
      oldSubscription: sub("https://push/old"),
      newSubscription: sub("https://push/new"),
    })
    expect(subscribe).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe(PUSH_RENEW_PATH)
    expect(init.credentials).toBe("same-origin")
    expect(JSON.parse(init.body as string)).toEqual({
      endpoint: "https://push/new",
      p256dh: "P",
      auth: "A",
      vapid_key: "-_8",
      old_endpoint: "https://push/old",
    })
  })

  it("subscribes with the old key when the event has no new subscription", async () => {
    const { fetch, subscribe } = await run(async () => new Response("{}"), {
      oldSubscription: sub("https://push/old"),
    })
    expect(subscribe).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it("throws nothing on a refusal or a network error", async () => {
    await run(async () => new Response('{"refused":true}', { status: 200 }), {
      oldSubscription: sub("https://push/old"),
      newSubscription: sub("https://push/new"),
    })
    await run(async () => Promise.reject(new TypeError("offline")), {
      oldSubscription: sub("https://push/old"),
      newSubscription: sub("https://push/new"),
    })
  })
})
