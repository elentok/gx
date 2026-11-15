import { act, cleanup, render, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"

const KEY = "gx.promptbox.big"

async function freshStore() {
  vi.resetModules()
  return import("./bigPromptStore")
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  localStorage.clear()
})

test("defaults to off with empty storage", async () => {
  const s = await freshStore()
  expect(s.getBigPrompt()).toBe(false)
})

test("reads true from storage at start", async () => {
  localStorage.setItem(KEY, "true")
  const s = await freshStore()
  expect(s.getBigPrompt()).toBe(true)
})

test("toggling writes storage and a fresh module sees it", async () => {
  const s = await freshStore()
  s.toggleBigPrompt()
  expect(localStorage.getItem(KEY)).toBe("true")
  const again = await freshStore()
  expect(again.getBigPrompt()).toBe(true)
})

test("throwing storage still toggles for the page", async () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("blocked")
  })
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("blocked")
  })
  const s = await freshStore()
  expect(s.getBigPrompt()).toBe(false)
  expect(() => s.toggleBigPrompt()).not.toThrow()
  expect(s.getBigPrompt()).toBe(true)
})

test("a storage event for the key updates the store and readers; other keys do nothing", async () => {
  const s = await freshStore()
  function Probe() {
    return <span data-testid="p">{String(s.useBigPrompt())}</span>
  }
  render(<Probe />)
  expect(screen.getByTestId("p").textContent).toBe("false")
  act(() => {
    window.dispatchEvent(new StorageEvent("storage", { key: "other", newValue: "true" }))
  })
  expect(screen.getByTestId("p").textContent).toBe("false")
  act(() => {
    window.dispatchEvent(new StorageEvent("storage", { key: KEY, newValue: "true" }))
  })
  expect(screen.getByTestId("p").textContent).toBe("true")
})
