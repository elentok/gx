import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { forceRefresh } from "./forceRefresh"
import { CACHE_PREFIX } from "./workerCore"

const showToast = vi.hoisted(() => vi.fn())
vi.mock("../ui/toaster", () => ({ showToast }))

const calls: string[] = []
let unregisterError: Error | null = null
const reload = vi.fn(() => void calls.push("reload"))

beforeEach(() => {
  calls.length = 0
  unregisterError = null
  reload.mockClear()
  showToast.mockClear()
  const registration = (id: string) => ({
    unregister: async () => {
      calls.push(`unregister ${id}`)
      if (unregisterError) throw unregisterError
      return true
    },
  })
  vi.stubGlobal("navigator", {
    serviceWorker: { getRegistrations: async () => [registration("a"), registration("b")] },
  })
  vi.stubGlobal("caches", {
    keys: async () => [`${CACHE_PREFIX}1`, `${CACHE_PREFIX}2`, "other-cache"],
    delete: async (n: string) => (calls.push(`delete ${n}`), true),
  })
  vi.stubGlobal("location", { reload })
})

afterEach(() => vi.unstubAllGlobals())

test("offline shows Can't reach gx and touches nothing", async () => {
  await forceRefresh(false)
  expect(showToast).toHaveBeenCalledWith({ type: "info", title: "Can't reach gx" })
  expect(calls).toEqual([])
})

test("online toasts, unregisters every registration, deletes only prefixed caches, then reloads once", async () => {
  await forceRefresh(true)
  expect(showToast).toHaveBeenCalledWith({
    type: "info",
    title: "Clearing the cache and reloading…",
  })
  expect(calls).toEqual([
    "unregister a",
    "unregister b",
    `delete ${CACHE_PREFIX}1`,
    `delete ${CACHE_PREFIX}2`,
    "reload",
  ])
})

test("a failing unregister still reloads", async () => {
  unregisterError = new Error("boom")
  await forceRefresh(true)
  expect(calls.filter((c) => c === "reload")).toHaveLength(1)
})
