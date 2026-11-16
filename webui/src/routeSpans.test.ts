import { beforeEach, expect, test, vi } from "vitest"
import {
  hardLoadDropped,
  hardLoadNavigationType,
  markReady,
  noteAuthScreen,
  noteCatchAll,
  noteSignedOut,
  resetRouteSpans,
  toNavigationType,
  trackRouter,
} from "./routeSpans"

const mocks = vi.hoisted(() => ({ queueSample: vi.fn() }))
vi.mock("./webPerf", () => ({ queueSample: mocks.queueSample }))

type State = Parameters<Parameters<typeof trackRouter>[0]["subscribe"]>[0]

// A router stub: `go` pushes a committed navigation to the subscriber.
function fakeRouter(path: string, initialized = true) {
  let listener: (state: State) => void = () => {}
  const state = (p: string, key: string, action: string, pattern = p) =>
    ({
      navigation: { state: "idle" },
      location: { key, pathname: p },
      historyAction: action,
      matches: [{ route: { path: pattern } }],
      initialized,
    }) as unknown as State
  const router = {
    state: state(path, "k0", "POP"),
    subscribe: (fn: (s: State) => void) => {
      listener = fn
      return () => {}
    },
  }
  let n = 0
  return {
    router: router as unknown as Parameters<typeof trackRouter>[0],
    go: (p: string, action = "PUSH", pattern = p) => listener(state(p, `k${++n}`, action, pattern)),
  }
}

function samples() {
  return mocks.queueSample.mock.calls.map((c) => c[0])
}

beforeEach(() => {
  resetRouteSpans()
  vi.clearAllMocks()
})

test("hard load reports app_ready from navigation start", () => {
  const { router } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  stop()
  expect(samples()).toHaveLength(1)
  expect(samples()[0]).toMatchObject({ metric: "app_ready", tab: "hard" })
})

test("soft route reports route_ready once, the first useReady call wins", () => {
  const { router, go } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  go("/stats")
  markReady("ready", "cache")
  markReady("error", "network")
  stop()
  expect(samples().map((s) => s.metric)).toEqual(["app_ready", "route_ready"])
  expect(samples()[1]).toMatchObject({
    tab: "route",
    trigger: "programmatic",
    data_source: "cache",
  })
})

test("a replace away from a known redirect source is one span for the final route", () => {
  const { router, go } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  go("/triage", "PUSH", "/triage/*")
  go("/epics", "REPLACE")
  markReady("ready", "network")
  stop()
  expect(samples().filter((s) => s.metric === "route_ready")).toHaveLength(1)
})

test("a replace away from another page is its own span", () => {
  const { router, go } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  go("/stats")
  markReady("ready", "network")
  go("/tips", "REPLACE")
  markReady("ready", "network")
  stop()
  expect(samples().filter((s) => s.metric === "route_ready")).toHaveLength(2)
})

test("the catch-all redirect after a soft navigation reports nothing", () => {
  const { router, go } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  noteCatchAll()
  go("/epics")
  markReady("ready", "network")
  go("/stats")
  markReady("ready", "network")
  stop()
  expect(samples().map((s) => s.metric)).toEqual(["app_ready", "route_ready"])
})

test("a catch-all flagged before the first loaders settle lands on the next commit", () => {
  const { router, go } = fakeRouter("/epics", false)
  const stop = trackRouter(router)
  noteCatchAll()
  go("/epics")
  markReady("ready", "network")
  stop()
  expect(samples()).toEqual([])
})

test("the first span after sign-out reports nothing, later ones do", () => {
  const { router, go } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  noteSignedOut()
  go("/stats")
  markReady("ready", "network")
  go("/tips")
  markReady("ready", "network")
  stop()
  expect(samples().map((s) => s.metric)).toEqual(["app_ready", "route_ready"])
})

test("an auth screen drops the hard load and the first span after it", () => {
  noteAuthScreen()
  expect(hardLoadDropped()).toBe(true)
  const { router } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  stop()
  expect(samples()).toEqual([])
})

test("a bfcache restore abandons the open span and marks the load", () => {
  const { router } = fakeRouter("/epics")
  const stop = trackRouter(router)
  window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }))
  markReady("ready", "network")
  stop()
  expect(samples()).toEqual([])
  expect(hardLoadNavigationType()).toBe("back-forward-cache")
})

test("a pointer press just before a navigation is its trigger", () => {
  const { router, go } = fakeRouter("/epics")
  const stop = trackRouter(router)
  markReady("ready", "network")
  document.dispatchEvent(new Event("pointerdown"))
  go("/stats")
  markReady("ready", "network")
  stop()
  expect(samples()[1]).toMatchObject({ trigger: "click" })
})

test("toNavigationType maps both spellings and prerender to navigate", () => {
  expect(toNavigationType("back_forward")).toBe("back-forward")
  expect(toNavigationType("back-forward")).toBe("back-forward")
  expect(toNavigationType("prerender")).toBe("navigate")
  expect(toNavigationType("reload")).toBe("reload")
  expect(toNavigationType("soft-navigation")).toBeUndefined()
  expect(toNavigationType(undefined)).toBeUndefined()
})
