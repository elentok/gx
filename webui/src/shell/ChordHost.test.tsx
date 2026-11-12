import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { Modal } from "../ui/modal/Modal"
import { renderThemed } from "../ui/render"
import { ChordHost } from "./ChordHost"
import type { Chord } from "./chords"

beforeEach(() => {
  vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const chord = (key: string, run: () => void): Chord => ({
  key,
  label: () => `Do ${key}`,
  run,
  available: () => true,
})

function setup(chords: Chord[], leader = "ctrl+e", extra: React.ReactNode = null) {
  const state = { snapshot: { leader_key: leader }, connected: true } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <>
            <input aria-label="field" />
            {extra}
            <ChordHost chords={chords} />
          </>
        ),
      },
    ],
    { initialEntries: ["/"] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

const leaderKey = (target: Window | Element = window) =>
  fireEvent.keyDown(target, { code: "KeyE", key: "e", ctrlKey: true })
const press = (code: string, init: KeyboardEventInit = {}) =>
  fireEvent.keyDown(window, { code, key: code, ...init })
const box = () => screen.queryByRole("menu")

test("Leader opens the box and the next key runs its chord and closes it", () => {
  const run = vi.fn()
  setup([chord("a", run)])
  leaderKey()
  expect(box()).toBeTruthy()
  press("KeyA")
  expect(run).toHaveBeenCalledTimes(1)
  expect(box()).toBeNull()
})

test("Leader in a field opens the box and the field keeps its text", () => {
  const run = vi.fn()
  setup([chord("a", run)])
  const field = screen.getByLabelText("field") as HTMLInputElement
  field.focus()
  fireEvent.change(field, { target: { value: "hi" } })
  expect(leaderKey(field)).toBe(false)
  expect(box()).toBeTruthy()
  expect(fireEvent.keyDown(field, { code: "KeyA", key: "a" })).toBe(false)
  expect(run).toHaveBeenCalledTimes(1)
  expect(field.value).toBe("hi")
})

test.each([
  ["Esc", () => press("Escape")],
  ["an unlisted key", () => press("KeyZ")],
  ["the Leader key", () => leaderKey()],
  ["a click outside", () => fireEvent.pointerDown(document.body)],
  ["window blur", () => fireEvent.blur(window)],
])("%s closes the box with no action", (_n, act_) => {
  const run = vi.fn()
  setup([chord("a", run)])
  leaderKey()
  act(act_)
  expect(box()).toBeNull()
  expect(run).not.toHaveBeenCalled()
})

test("a click inside the box does not close it by itself", () => {
  setup([chord("a", () => {})])
  leaderKey()
  fireEvent.pointerDown(screen.getByRole("menu"))
  expect(box()).toBeTruthy()
})

test("a route change closes the box", async () => {
  const router = setup([chord("a", () => {})])
  leaderKey()
  await act(() => router.navigate("/other"))
  expect(box()).toBeNull()
})

test("a taken key never reaches another window listener", () => {
  const other = vi.fn()
  window.addEventListener("keydown", other)
  setup([chord("a", () => {})])
  leaderKey()
  press("KeyK", { ctrlKey: true })
  window.removeEventListener("keydown", other)
  expect(other).not.toHaveBeenCalled()
})

test("with a layer open the Leader key opens nothing", () => {
  setup(
    [chord("a", () => {})],
    "ctrl+e",
    <Modal label="Over" onClose={() => {}}>
      <p>body</p>
    </Modal>,
  )
  leaderKey()
  expect(box()).toBeNull()
})

test("the box stays open with no timer", () => {
  vi.useFakeTimers()
  setup([chord("a", () => {})])
  leaderKey()
  act(() => {
    vi.advanceTimersByTime(60 * 60 * 1000)
  })
  expect(box()).toBeTruthy()
})

test("an empty Leader key means no box", () => {
  setup([chord("a", () => {})], "")
  leaderKey()
  expect(box()).toBeNull()
})
