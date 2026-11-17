import { renderHook } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { useTwoFingerSwipeDown } from "./useTwoFingerSwipeDown"

function touch(type: string, pts: [number, number][]) {
  const e = new Event(type, { bubbles: true, cancelable: true }) as Event & { touches: unknown }
  e.touches = pts.map(([clientX, clientY]) => ({ clientX, clientY }))
  document.dispatchEvent(e)
}

function setup() {
  Object.assign(window, { innerWidth: 1000, innerHeight: 1000 })
  const onSwipe = vi.fn()
  renderHook(() => useTwoFingerSwipeDown(onSwipe))
  return onSwipe
}

afterEach(() => vi.restoreAllMocks())

test("a two-finger swipe down in the center fires once", () => {
  const onSwipe = setup()
  touch("touchstart", [[450, 400], [550, 400]])
  touch("touchmove", [[450, 450], [550, 450]])
  touch("touchmove", [[450, 520], [550, 520]])
  touch("touchmove", [[450, 600], [550, 600]])
  expect(onSwipe).toHaveBeenCalledTimes(1)
})

test("a swipe that starts outside the center does nothing", () => {
  const onSwipe = setup()
  touch("touchstart", [[450, 50], [550, 50]])
  touch("touchmove", [[450, 300], [550, 300]])
  expect(onSwipe).not.toHaveBeenCalled()
})

test("one finger, swiping up and pinching do nothing", () => {
  const onSwipe = setup()
  touch("touchstart", [[500, 400]])
  touch("touchmove", [[500, 600]])
  touch("touchstart", [[450, 600], [550, 600]])
  touch("touchmove", [[450, 400], [550, 400]])
  touch("touchstart", [[450, 400], [550, 400]])
  touch("touchmove", [[350, 500], [650, 500]])
  expect(onSwipe).not.toHaveBeenCalled()
})
