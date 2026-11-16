import { renderHook, screen } from "@testing-library/react"
import { createElement } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { postAfk, toggleAfk } from "./afk"
import { HEARTBEAT_INTERVAL_MS, useHeartbeat } from "./rest"
import { renderThemed } from "./ui/render"

describe("useHeartbeat", () => {
  let fetchMock: ReturnType<typeof vi.fn>
  beforeEach(() => {
    vi.useFakeTimers()
    fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal("fetch", fetchMock)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    setVisibility("visible")
  })

  function setVisibility(state: "visible" | "hidden") {
    Object.defineProperty(document, "visibilityState", { value: state, configurable: true })
    document.dispatchEvent(new Event("visibilitychange"))
  }
  const minute = () => vi.advanceTimersByTimeAsync(HEARTBEAT_INTERVAL_MS)

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
})

describe("postAfk", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("posts the wanted mode as JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal("fetch", fetchMock)
    await postAfk(true)
    expect(fetchMock).toHaveBeenCalledWith("/v1/afk", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: '{"on":true}',
    })
  })

  it("rejects when the server refuses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not allowed", { status: 403 })))
    await expect(postAfk(false)).rejects.toThrow("not allowed")
  })

  it("shows an error toast when the write fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("AFK is locked", { status: 500 })),
    )
    renderThemed(createElement("div"))
    toggleAfk(false)
    expect(await screen.findByText("AFK is locked")).toBeTruthy()
  })
})
