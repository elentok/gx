import { renderHook } from "@testing-library/react"
import { expect, it, vi } from "vitest"
import { useOnCloseSignal } from "./closeSignal"

it("calls onClose when the signal changes, not on mount or on an unrelated re-render", () => {
  const first = vi.fn()
  const second = vi.fn()
  const { rerender } = renderHook(({ signal, cb }) => useOnCloseSignal(signal, cb), {
    initialProps: { signal: 0, cb: first },
  })
  expect(first).not.toHaveBeenCalled()
  rerender({ signal: 0, cb: second })
  expect(first).not.toHaveBeenCalled()
  expect(second).not.toHaveBeenCalled()
  rerender({ signal: 1, cb: second })
  expect(first).not.toHaveBeenCalled()
  expect(second).toHaveBeenCalledTimes(1)
})
