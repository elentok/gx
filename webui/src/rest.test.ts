import { renderHook } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { HEARTBEAT_INTERVAL_MS, useHeartbeat, useRestHeartbeat } from "./rest"

const { snapshot, showToast } = vi.hoisted(() => ({
  snapshot: { current: undefined as unknown },
  showToast: vi.fn(),
}))

vi.mock("./connection", () => ({
  useConnectionContext: () => undefined,
  useConnectionState: () => ({ snapshot: snapshot.current }),
}))
vi.mock("./ui/toaster", () => ({ showToast }))

let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => {
  vi.useFakeTimers()
  fetchMock = vi.fn().mockResolvedValue({ ok: true })
  vi.stubGlobal("fetch", fetchMock)
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  showToast.mockReset()
  snapshot.current = undefined
  setVisibility("visible")
})

function setVisibility(state: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { value: state, configurable: true })
  document.dispatchEvent(new Event("visibilitychange"))
}
const minute = () => vi.advanceTimersByTimeAsync(HEARTBEAT_INTERVAL_MS)

describe("useHeartbeat", () => {
  it("sends one request per minute, only after input", async () => {
    renderHook(() => useHeartbeat(true))
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
    window.dispatchEvent(new Event("keydown"))
    window.dispatchEvent(new Event("pointerdown"))
    window.dispatchEvent(new Event("scroll"))
    expect(fetchMock).not.toHaveBeenCalled()
    await minute()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/v1/rest/active")
    await minute()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("ignores other input", async () => {
    renderHook(() => useHeartbeat(true))
    window.dispatchEvent(new Event("mousemove"))
    window.dispatchEvent(new Event("keyup"))
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("stops the timer while the tab is hidden and resumes when it shows", async () => {
    renderHook(() => useHeartbeat(true))
    setVisibility("hidden")
    expect(vi.getTimerCount()).toBe(0)
    window.dispatchEvent(new Event("keydown"))
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
    setVisibility("visible")
    expect(vi.getTimerCount()).toBe(1)
    await minute()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("attaches nothing while off", async () => {
    renderHook(() => useHeartbeat(false))
    window.dispatchEvent(new Event("keydown"))
    expect(vi.getTimerCount()).toBe(0)
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("detaches when turned off", async () => {
    const { rerender } = renderHook(({ on }) => useHeartbeat(on), { initialProps: { on: true } })
    window.dispatchEvent(new Event("keydown"))
    rerender({ on: false })
    expect(vi.getTimerCount()).toBe(0)
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("shows no toast when the beat fails", async () => {
    fetchMock.mockResolvedValue(new Response("down", { status: 500 }))
    renderHook(() => useHeartbeat(true))
    window.dispatchEvent(new Event("keydown"))
    await minute()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(showToast).not.toHaveBeenCalled()
  })

  it("shows no toast when the server is unreachable", async () => {
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"))
    renderHook(() => useHeartbeat(true))
    window.dispatchEvent(new Event("keydown"))
    await minute()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(showToast).not.toHaveBeenCalled()
  })
})

describe("useRestHeartbeat", () => {
  it("sends beats while counting and no Rest period shows", async () => {
    snapshot.current = { rest: { counting: true } }
    renderHook(() => useRestHeartbeat())
    window.dispatchEvent(new Event("keydown"))
    await minute()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("sends nothing while a Rest period shows", async () => {
    snapshot.current = { rest: { counting: true, period: { kind: "break" } } }
    renderHook(() => useRestHeartbeat())
    window.dispatchEvent(new Event("keydown"))
    expect(vi.getTimerCount()).toBe(0)
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("sends nothing while not counting", async () => {
    snapshot.current = { rest: { counting: false } }
    renderHook(() => useRestHeartbeat())
    window.dispatchEvent(new Event("keydown"))
    await minute()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
