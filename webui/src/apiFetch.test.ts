import { afterEach, expect, test, vi } from "vitest"
import { apiFetch, apiFetchSide, onSignedOut } from "./apiFetch"
import * as routeSpans from "./routeSpans"
import { readPageCache, writePageCache } from "./pageCache"
import { sessionSnapshotCache } from "./snapshotCache"
import type { Snapshot } from "./snapshot"

afterEach(() => {
  vi.unstubAllGlobals()
  sessionStorage.clear()
})

test("a 401 clears the page caches and fires the signed-out signal", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("", { status: 401 })),
  )
  writePageCache("epics", { a: 1 })
  sessionSnapshotCache.save({ seq: 1 } as Snapshot)
  const signal = vi.fn()
  const off = onSignedOut(signal)

  const res = await apiFetch("/v1/epics")

  expect(res.status).toBe(401)
  expect(signal).toHaveBeenCalledTimes(1)
  expect(readPageCache("epics")).toBeNull()
  expect(sessionSnapshotCache.load()).toBeNull()
  off()
})

test("a side fetch is not counted as an API request, and a plain one is", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 200 })),
  )
  const note = vi.spyOn(routeSpans, "noteApiRequest")

  await apiFetchSide("/v1/epics")
  await apiFetch("/v1/snapshot")

  expect(note.mock.calls).toEqual([[true], [false]])
  note.mockRestore()
})

test("a 200 passes through untouched", async () => {
  const ok = new Response("{}", { status: 200 })
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ok),
  )
  writePageCache("epics", { a: 1 })
  const signal = vi.fn()
  const off = onSignedOut(signal)

  expect(await apiFetch("/v1/epics")).toBe(ok)

  expect(signal).not.toHaveBeenCalled()
  expect(readPageCache("epics")).toEqual({ a: 1 })
  off()
})
