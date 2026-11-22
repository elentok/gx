import { renderHook } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import { useLatest, useOnChange } from "./hooks"

test("useLatest returns one ref holding the newest value without re-rendering", () => {
  let renders = 0
  const { result, rerender } = renderHook(
    ({ v }) => {
      renders++
      return useLatest(v)
    },
    { initialProps: { v: 1 } },
  )
  const ref = result.current
  expect(ref.current).toBe(1)
  rerender({ v: 2 })
  expect(result.current).toBe(ref)
  expect(ref.current).toBe(2)
  expect(renders).toBe(2)
})

test("useOnChange runs on a change and not on mount or an equal value", () => {
  const fn = vi.fn()
  const { rerender } = renderHook(({ v }) => useOnChange(v, fn), { initialProps: { v: "a" } })
  expect(fn).not.toHaveBeenCalled()
  rerender({ v: "a" })
  expect(fn).not.toHaveBeenCalled()
  rerender({ v: "b" })
  expect(fn).toHaveBeenCalledExactlyOnceWith("b")
})

test("useOnChange with immediate also runs on mount", () => {
  const fn = vi.fn()
  const { rerender } = renderHook(({ v }) => useOnChange(v, fn, { immediate: true }), {
    initialProps: { v: "a" },
  })
  expect(fn).toHaveBeenCalledExactlyOnceWith("a")
  rerender({ v: "b" })
  expect(fn).toHaveBeenCalledTimes(2)
})

test("useOnChange watches each element of a tuple, not its identity", () => {
  const fn = vi.fn()
  const { rerender } = renderHook(({ a, b }) => useOnChange([a, b], fn), {
    initialProps: { a: 1, b: 1 },
  })
  rerender({ a: 1, b: 1 })
  expect(fn).not.toHaveBeenCalled()
  rerender({ a: 1, b: 2 })
  expect(fn).toHaveBeenCalledTimes(1)
})

test("useOnChange runs the returned cleanup before the next change and on unmount only", () => {
  const cleanup = vi.fn()
  const { rerender, unmount } = renderHook(
    ({ v }) => useOnChange(v, () => cleanup, { immediate: true }),
    {
      initialProps: { v: 1 },
    },
  )
  rerender({ v: 1 })
  expect(cleanup).not.toHaveBeenCalled()
  rerender({ v: 2 })
  expect(cleanup).toHaveBeenCalledTimes(1)
  unmount()
  expect(cleanup).toHaveBeenCalledTimes(2)
})

test("useOnChange calls the newest callback", () => {
  const first = vi.fn()
  const second = vi.fn()
  const { rerender } = renderHook(({ v, fn }) => useOnChange(v, fn), {
    initialProps: { v: 1, fn: first },
  })
  rerender({ v: 1, fn: second })
  rerender({ v: 2, fn: second })
  expect(first).not.toHaveBeenCalled()
  expect(second).toHaveBeenCalledWith(2)
})
