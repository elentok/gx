import { act, renderHook } from "@testing-library/react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { useKeyboardOpen } from "./useKeyboardOpen"

class FakeViewport extends EventTarget {
  height = 800
  scale = 1
  resize(height: number) {
    this.height = height
    this.dispatchEvent(new Event("resize"))
  }
}

let vv: FakeViewport
let a: HTMLInputElement
let b: HTMLInputElement
let button: HTMLButtonElement
let checkbox: HTMLInputElement

beforeEach(() => {
  vv = new FakeViewport()
  Object.defineProperty(window, "visualViewport", { value: vv, configurable: true })
  Object.defineProperty(window, "innerHeight", { value: 800, configurable: true })
  a = document.createElement("input")
  b = document.createElement("textarea") as unknown as HTMLInputElement
  button = document.createElement("button")
  checkbox = document.createElement("input")
  checkbox.type = "checkbox"
  document.body.append(a, b, button, checkbox)
})

afterEach(() => {
  document.body.replaceChildren()
  Object.defineProperty(window, "visualViewport", { value: undefined, configurable: true })
})

const mount = () => renderHook(() => useKeyboardOpen())

test("open with a large gap and a focused text field", () => {
  const { result } = mount()
  act(() => a.focus())
  act(() => vv.resize(450))
  expect(result.current).toBe(true)
})

test("closed with a gap but no focus", () => {
  const { result } = mount()
  act(() => vv.resize(450))
  expect(result.current).toBe(false)
})

test("closed with focus but a gap under 150px", () => {
  const { result } = mount()
  act(() => a.focus())
  act(() => vv.resize(700))
  expect(result.current).toBe(false)
})

test("closed while pinch-zoomed with no keyboard", () => {
  const { result } = mount()
  act(() => a.focus())
  vv.scale = 2
  act(() => vv.resize(400)) // half the window tall in zoomed pixels: the whole screen
  expect(result.current).toBe(false)
})

test("open when iOS zooms into the focused field and the keyboard is up", () => {
  const { result } = mount()
  act(() => a.focus())
  vv.scale = 1.25
  act(() => vv.resize(400)) // 400 zoomed pixels is 500 screen pixels: 300 hidden by the keyboard
  expect(result.current).toBe(true)
})

test("a focused button or checkbox does not count", () => {
  const { result } = mount()
  act(() => vv.resize(450))
  act(() => button.focus())
  expect(result.current).toBe(false)
  act(() => checkbox.focus())
  expect(result.current).toBe(false)
})

test("focusin opens and focusout closes", () => {
  const { result } = mount()
  act(() => vv.resize(450))
  act(() => a.focus())
  expect(result.current).toBe(true)
  act(() => a.blur())
  expect(result.current).toBe(false)
})

test("a resize arriving after focusout still settles closed", () => {
  const { result } = mount()
  act(() => a.focus())
  act(() => vv.resize(450))
  act(() => a.blur())
  act(() => vv.resize(800))
  expect(result.current).toBe(false)
})

test("a late resize after focusin opens", () => {
  const { result } = mount()
  act(() => a.focus())
  expect(result.current).toBe(false)
  act(() => vv.resize(450))
  expect(result.current).toBe(true)
})

test("moving focus between fields never reports closed", () => {
  const { result } = mount()
  act(() => vv.resize(450))
  act(() => a.focus())
  act(() => {
    // focusout carries the next field as relatedTarget while activeElement is already body.
    a.dispatchEvent(new FocusEvent("focusout", { bubbles: true, relatedTarget: b }))
  })
  expect(result.current).toBe(true)
  act(() => b.focus())
  expect(result.current).toBe(true)
})

test("false without a visual viewport", () => {
  Object.defineProperty(window, "visualViewport", { value: undefined, configurable: true })
  const { result } = mount()
  act(() => a.focus())
  expect(result.current).toBe(false)
})

test("a keyboard size that iOS never reports by event is still picked up while a field is focused", () => {
  vi.useFakeTimers()
  try {
    const { result } = mount()
    act(() => a.focus())
    vv.height = 450 // no resize event
    expect(result.current).toBe(false)
    act(() => vi.advanceTimersByTime(300))
    expect(result.current).toBe(true)
    act(() => a.blur())
    expect(result.current).toBe(false)
  } finally {
    vi.useRealTimers()
  }
})
