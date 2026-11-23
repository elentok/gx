import { act, render } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { useAppBadge } from "./appBadge"
import { usePushDevice } from "./deviceState"
import { useConnectionState } from "../connection"

vi.mock("../connection", () => ({
  useConnectionContext: () => ({}),
  useConnectionState: vi.fn(),
}))
vi.mock("./deviceState", () => ({ usePushDevice: vi.fn() }))

const setAppBadge = vi.fn()
const clearAppBadge = vi.fn()

function Probe() {
  useAppBadge()
  return null
}

function state(badge: number | undefined, liveSnapshot = true) {
  vi.mocked(useConnectionState).mockReturnValue({
    snapshot: badge === undefined ? {} : { badge },
    liveSnapshot,
  } as never)
}

function device(on: boolean) {
  vi.mocked(usePushDevice).mockReturnValue({ on } as never)
}

function setVisibility(value: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { value, configurable: true })
}

describe("useAppBadge", () => {
  beforeEach(() => {
    Object.assign(navigator, {
      setAppBadge: setAppBadge.mockResolvedValue(undefined),
      clearAppBadge: clearAppBadge.mockResolvedValue(undefined),
    })
  })
  afterEach(() => {
    vi.clearAllMocks()
    Reflect.deleteProperty(navigator, "setAppBadge")
    Reflect.deleteProperty(navigator, "clearAppBadge")
    Reflect.deleteProperty(document, "visibilityState")
  })

  it("with the switch On, sets the badge on each live snapshot change and clears at 0", () => {
    device(true)
    state(3)
    const { rerender } = render(<Probe />)
    expect(setAppBadge).toHaveBeenLastCalledWith(3)
    state(5)
    rerender(<Probe />)
    expect(setAppBadge).toHaveBeenLastCalledWith(5)
    state(0)
    rerender(<Probe />)
    expect(clearAppBadge).toHaveBeenCalledTimes(1)
  })

  it("treats a live snapshot without a badge field as 0, also on return to the foreground", () => {
    device(true)
    state(undefined)
    render(<Probe />)
    expect(clearAppBadge).toHaveBeenCalledTimes(1)
    clearAppBadge.mockClear()
    setVisibility("visible")
    act(() => void document.dispatchEvent(new Event("visibilitychange")))
    expect(clearAppBadge).toHaveBeenCalledTimes(1)
    expect(setAppBadge).not.toHaveBeenCalled()
  })

  it("ignores a cached snapshot", () => {
    device(true)
    state(3, false)
    render(<Probe />)
    expect(setAppBadge).not.toHaveBeenCalled()
    expect(clearAppBadge).not.toHaveBeenCalled()
  })

  it("returning to the foreground re-applies the last number", () => {
    device(true)
    state(4)
    render(<Probe />)
    setAppBadge.mockClear()
    setVisibility("hidden")
    act(() => void document.dispatchEvent(new Event("visibilitychange")))
    expect(setAppBadge).not.toHaveBeenCalled()
    setVisibility("visible")
    act(() => void document.dispatchEvent(new Event("visibilitychange")))
    expect(setAppBadge).toHaveBeenCalledWith(4)
  })

  it("with the switch Off, never touches the badge", () => {
    device(false)
    state(3)
    render(<Probe />)
    act(() => void document.dispatchEvent(new Event("visibilitychange")))
    expect(setAppBadge).not.toHaveBeenCalled()
    expect(clearAppBadge).not.toHaveBeenCalled()
  })

  it("with no support, does nothing and does not throw", () => {
    Reflect.deleteProperty(navigator, "setAppBadge")
    Reflect.deleteProperty(navigator, "clearAppBadge")
    device(true)
    state(3)
    expect(() => render(<Probe />)).not.toThrow()
  })

  it("swallows a rejected setAppBadge", async () => {
    setAppBadge.mockRejectedValue(new Error("denied"))
    device(true)
    state(3)
    render(<Probe />)
    await Promise.resolve()
    expect(setAppBadge).toHaveBeenCalledWith(3)
  })
})
