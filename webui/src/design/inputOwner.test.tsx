import { renderHook } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { useInputOwner, type SubscribeOwner, type TakeOver } from "./inputOwner"

afterEach(() => {
  vi.unstubAllGlobals()
})

const takeOver: TakeOver = async () => ({})

function setup(enabled: boolean) {
  const subscribe = vi.fn<SubscribeOwner>(() => () => {})
  renderHook(() => useInputOwner({ target: "s-a", enabled, subscribe, takeOver }))
  return subscribe
}

test("a page without crypto.randomUUID still gets a client id", () => {
  vi.stubGlobal("crypto", { getRandomValues: crypto.getRandomValues.bind(crypto) })
  const subscribe = setup(true)
  expect(subscribe).toHaveBeenCalledTimes(1)
  expect(subscribe.mock.calls[0][1]).toMatch(/^web-[0-9a-f]{32}$/)
})

test("two hook users in one tab share one client id", () => {
  const a = setup(true)
  const b = setup(true)
  expect(a.mock.calls[0][1]).toBe(b.mock.calls[0][1])
})

test("a disabled hook opens no attach", () => {
  const subscribe = setup(false)
  expect(subscribe).not.toHaveBeenCalled()
})
