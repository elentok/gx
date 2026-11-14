import { act, renderHook, waitFor } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { useKeyedFetch } from "./useKeyedFetch"

function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe("useKeyedFetch", () => {
  it("returns the value of the current key", async () => {
    const { result } = renderHook(() => useKeyedFetch("a", () => Promise.resolve(1)))
    expect(result.current).toBeUndefined()
    await waitFor(() => expect(result.current).toEqual({ value: 1 }))
  })

  it("hides the old result when the key changes and ignores a stale answer", async () => {
    const first = deferred<string>()
    const second = deferred<string>()
    const calls = { a: first, b: second }
    const { result, rerender } = renderHook(
      ({ k }: { k: "a" | "b" }) => useKeyedFetch(k, () => calls[k].promise),
      { initialProps: { k: "a" } },
    )
    rerender({ k: "b" })
    await act(async () => first.resolve("stale"))
    expect(result.current).toBeUndefined()
    await act(async () => second.resolve("fresh"))
    expect(result.current).toEqual({ value: "fresh" })
  })

  it("reports a failure", async () => {
    const { result } = renderHook(() => useKeyedFetch("a", () => Promise.reject(new Error("x"))))
    await waitFor(() => expect(result.current).toEqual({ failed: true }))
  })

  it("does not fetch for a null key", () => {
    let calls = 0
    const { result } = renderHook(() =>
      useKeyedFetch(null, () => {
        calls++
        return Promise.resolve(1)
      }),
    )
    expect(result.current).toBeUndefined()
    expect(calls).toBe(0)
  })
})
