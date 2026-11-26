import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { setViMode, useViMode } from "./vimode"

describe("useViMode", () => {
  afterEach(() => vi.restoreAllMocks())

  it("is off by default", () => {
    expect(renderHook(() => useViMode()).result.current).toBe(false)
  })

  it("follows a change in this tab and keeps it in localStorage", () => {
    const { result } = renderHook(() => useViMode())
    act(() => setViMode(true))
    expect(result.current).toBe(true)
    expect(localStorage.getItem("gx.vi-mode")).toBe("on")
    act(() => setViMode(false))
    expect(result.current).toBe(false)
  })

  it("follows a change from another tab", () => {
    const { result } = renderHook(() => useViMode())
    act(() => {
      localStorage.setItem("gx.vi-mode", "on")
      window.dispatchEvent(new StorageEvent("storage", { key: "gx.vi-mode" }))
    })
    expect(result.current).toBe(true)
  })

  it("reads as off when storage throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked")
    })
    expect(renderHook(() => useViMode()).result.current).toBe(false)
  })
})
