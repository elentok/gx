import { act, renderHook } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { installFakeNavigation } from "./fakeNavigation.testutil"
import { useHistoryState } from "./useHistoryState"

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

test("without window.navigation nothing is supported or possible", () => {
  expect(renderHook(useHistoryState).result.current).toEqual({
    supported: false,
    canGoBack: false,
    canGoForward: false,
  })
})

test.each([
  { canGoBack: true, canGoForward: false },
  { canGoBack: false, canGoForward: true },
])("it mirrors $canGoBack back and $canGoForward forward", (state) => {
  installFakeNavigation(state)
  expect(renderHook(useHistoryState).result.current).toEqual({ supported: true, ...state })
})

test("currententrychange updates both values until unmount removes the listeners", () => {
  const nav = installFakeNavigation()
  const added = vi.spyOn(window.navigation, "addEventListener")
  const removed = vi.spyOn(window.navigation, "removeEventListener")
  const { result, unmount } = renderHook(useHistoryState)

  act(() => nav.set({ canGoBack: true, canGoForward: true }))
  expect(result.current).toEqual({ supported: true, canGoBack: true, canGoForward: true })
  act(() => nav.set({ canGoBack: false, canGoForward: false }))
  expect(result.current).toEqual({ supported: true, canGoBack: false, canGoForward: false })

  unmount()
  expect(added).toHaveBeenCalled()
  const listeners = (calls: unknown[][]) => calls.map(([type, listener]) => [type, listener])
  expect(listeners(removed.mock.calls)).toEqual(listeners(added.mock.calls))
})
