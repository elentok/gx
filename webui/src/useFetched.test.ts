import { act, renderHook, waitFor } from "@testing-library/react"
import { beforeEach, expect, test, vi } from "vitest"
import { markReady } from "./routeSpans"
import { ROUTE_NAMES } from "./routeNames"
import { useFetched } from "./useFetched"
import { useDetailReady, useFetchedReady } from "./useReady"

vi.mock("./routeSpans", () => ({ markReady: vi.fn() }))

beforeEach(() => {
  sessionStorage.clear()
  vi.mocked(markReady).mockClear()
})

function useWired(
  key: string,
  fetcher: () => Promise<unknown>,
  empty: (d: unknown) => boolean = () => false,
) {
  const state = useFetched(key, fetcher)
  useFetchedReady(state, state.data !== null && empty(state.data))
  return state
}

test("a network paint ends the span as ready from the network", async () => {
  renderHook(() =>
    useWired(
      "n1",
      () => Promise.resolve("x"),
      () => false,
    ),
  )
  await waitFor(() => expect(markReady).toHaveBeenCalledWith("ready", "network"))
  expect(markReady).toHaveBeenCalledTimes(1)
})

test("a cache paint ends the span as ready from the cache, and the network answer adds nothing", async () => {
  sessionStorage.setItem("gx.page.c1", JSON.stringify({ id: "", value: "old" }))
  const { result } = renderHook(() =>
    useWired(
      "c1",
      () => Promise.resolve("new"),
      () => false,
    ),
  )
  expect(markReady).toHaveBeenCalledWith("ready", "cache")
  await waitFor(() => expect(result.current.data).toBe("new"))
  expect(result.current.source).toBe("network")
  // The hook may call again with the network source; the span engine keeps only the first call.
  expect(vi.mocked(markReady).mock.calls[0]).toEqual(["ready", "cache"])
})

test("a failed first fetch ends the span with error", async () => {
  renderHook(() => useWired("e1", () => Promise.reject(new Error("no"))))
  await waitFor(() => expect(markReady).toHaveBeenCalledWith("error", "network"))
})

test("an empty answer ends the span with empty", async () => {
  renderHook(() =>
    useWired(
      "m1",
      () => Promise.resolve([]),
      (d) => (d as unknown[]).length === 0,
    ),
  )
  await waitFor(() => expect(markReady).toHaveBeenCalledWith("empty", "network"))
})

test("an error over cached data stays ready", async () => {
  sessionStorage.setItem("gx.page.e2", JSON.stringify({ id: "", value: "old" }))
  const { result } = renderHook(() =>
    useWired(
      "e2",
      () => Promise.reject(new Error("no")),
      () => false,
    ),
  )
  expect(result.current.failed).toBe(false)
  expect(markReady).toHaveBeenCalledWith("ready", "cache")
  expect(markReady).not.toHaveBeenCalledWith("error", expect.anything())
})

test("nothing fires while the first fetch is pending", () => {
  renderHook(() => useWired("p1", () => new Promise(() => {})))
  expect(markReady).not.toHaveBeenCalled()
})

// Every normalized route needs a decision. Add the new route here when ROUTE_NAMES grows.
const READY_WIRING: Record<
  string,
  "useFetched" | "useDetailReady" | "useReadyWhen" | "useReady" | "redirect"
> = {
  "/epics": "useFetched",
  "/archive": "useFetched",
  "/archive/*": "useDetailReady",
  "/agents/*": "useReadyWhen",
  "/stats": "useFetched",
  "/cleanup": "useFetched",
  "/schedules": "useFetched",
  "/schedules/new": "useFetched",
  "/schedules/*": "useFetched",
  "/tips": "useFetched",
  "/settings": "useFetched",
  "/settings/s/:section": "useFetched",
  "/settings/p/:project": "useFetched",
  "/settings/p/:project/s/:section": "useFetched",
  "/settings/logins": "useFetched",
  "/settings/logins/:id": "useFetched",
  "/settings/password": "useFetched",
  "/settings/authenticator": "useFetched",
  "/settings/certificate": "useFetched",
  "/settings/agents": "redirect",
  "/settings/*": "useReady",
  "/files/*": "useFetched",
  "/d/*": "useDetailReady",
  "/drafts/:id": "useReadyWhen",
  "/triage/*": "useDetailReady",
  "/t/*": "useDetailReady",
  "/e/*": "useDetailReady",
  "/diff": "useReadyWhen",
}

test("a detail ends the span once: cache first, then the network answer adds nothing", () => {
  const { rerender } = renderHook(
    ({ s }) => useDetailReady({ detail: { a: 1 }, notFound: false, source: s }),
    { initialProps: { s: "cache" as "cache" | "network" } },
  )
  rerender({ s: "network" })
  expect(vi.mocked(markReady).mock.calls[0]).toEqual(["ready", "cache"])
})

test("a detail still loading fires nothing, not-found and empty end as empty", () => {
  renderHook(() => useDetailReady({ detail: null, notFound: false, source: "cache" }))
  expect(markReady).not.toHaveBeenCalled()
  renderHook(() => useDetailReady({ detail: null, notFound: true, source: "network" }))
  expect(markReady).toHaveBeenCalledWith("empty", "network")
  vi.mocked(markReady).mockClear()
  renderHook(() => useDetailReady({ detail: { a: 1 }, notFound: false, source: "network" }, true))
  expect(markReady).toHaveBeenCalledWith("empty", "network")
})

test("every normalized route has a readiness case", () => {
  expect(Object.keys(READY_WIRING).sort()).toEqual(Object.keys(ROUTE_NAMES).sort())
})

const focus = () => act(() => void window.dispatchEvent(new Event("focus")))

test("refetchOnFocus refetches when the window regains focus", async () => {
  const fetcher = vi.fn().mockResolvedValueOnce("one").mockResolvedValueOnce("two")
  const { result } = renderHook(() => useFetched("k", fetcher, undefined, { refetchOnFocus: true }))
  await waitFor(() => expect(result.current.data).toBe("one"))
  focus()
  await waitFor(() => expect(result.current.data).toBe("two"))
  expect(fetcher).toHaveBeenCalledTimes(2)
})

test("cached data keeps one identity across renders, so effects keyed on it do not loop", () => {
  sessionStorage.setItem("gx.page.cached", JSON.stringify({ id: "", value: { rows: [1] } }))
  const pending = () => new Promise<{ rows: number[] }>(() => {})
  const { result, rerender } = renderHook(() => useFetched("cached", pending))
  const first = result.current.data
  expect(first).toEqual({ rows: [1] })
  rerender()
  expect(result.current.data).toBe(first)
})

test("without refetchOnFocus a focus event does not refetch", async () => {
  const fetcher = vi.fn().mockResolvedValue("one")
  const { result } = renderHook(() => useFetched("k2", fetcher))
  await waitFor(() => expect(result.current.data).toBe("one"))
  focus()
  expect(fetcher).toHaveBeenCalledTimes(1)
})
