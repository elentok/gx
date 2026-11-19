import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { Modal } from "../ui/modal/Modal"
import { renderThemed } from "../ui/render"
import { ChordHost } from "./ChordHost"
import { getBigPrompt, setBigPrompt } from "../ui/expandable-text/bigPromptStore"
import { chordTable, type Chord } from "./chords"
import { registerHistoryOpener } from "../history/historyOpener"
import { registerChatDraftPage, registerDraftPage } from "./draftPageActions"
import { closeRename, useRenameRequest } from "./renameOpen"

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

test("Shift makes the capital chord, apart from the lowercase one", () => {
  const lower = vi.fn()
  const upper = vi.fn()
  setup([chord("c", lower), chord("C", upper)])
  leaderKey()
  press("KeyC", { shiftKey: true })
  expect([lower.mock.calls.length, upper.mock.calls.length]).toEqual([0, 1])
  leaderKey()
  press("KeyC")
  expect([lower.mock.calls.length, upper.mock.calls.length]).toEqual([1, 1])
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

test("a bare modifier keydown leaves the box open, so Cmd+K still finds it open", () => {
  const other = vi.fn()
  setup([chord("a", () => {})])
  leaderKey()
  fireEvent.keyDown(window, { code: "ControlLeft", key: "Control", ctrlKey: true })
  expect(box()).toBeTruthy()
  window.addEventListener("keydown", other)
  press("KeyK", { ctrlKey: true })
  window.removeEventListener("keydown", other)
  expect(box()).toBeNull()
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

describe("Leader then b", () => {
  const table = () =>
    chordTable({ remove: () => {}, rename: () => {}, switchModel: () => {}, history: () => {} })
  afterEach(() => setBigPrompt(false))

  test("flips big mode from a focused textarea and the text stays", () => {
    setup(table(), "ctrl+e", <textarea aria-label="Message" />)
    const field = screen.getByLabelText("Message") as HTMLTextAreaElement
    field.focus()
    fireEvent.change(field, { target: { value: "hi" } })
    leaderKey(field)
    fireEvent.keyDown(field, { code: "KeyB", key: "b" })
    expect(getBigPrompt()).toBe(true)
    expect(field.value).toBe("hi")
  })

  test("does nothing with a layer open", () => {
    setup(
      table(),
      "ctrl+e",
      <Modal label="Over" onClose={() => {}}>
        <p>body</p>
      </Modal>,
    )
    leaderKey()
    press("KeyB")
    expect(box()).toBeNull()
    expect(getBigPrompt()).toBe(false)
  })
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

function RenameProbe() {
  const request = useRenameRequest()
  return (
    <p data-testid="renaming">
      {request ? ("session" in request ? request.session.id : "draft") : "none"}
    </p>
  )
}

function setupChats(path: string, sessions = [session("s1", "gamma"), session("s2", "gamma")]) {
  const state = {
    snapshot: { leader_key: "ctrl+e", sessions },
    connected: true,
  } as unknown as ConnectionState
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
            <textarea aria-label="Message" defaultValue="" />
            <RenameProbe />
            <ChordHost />
          </>
        ),
      },
    ],
    { initialEntries: [path] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
}

test("{leader} r on a chat page renames that chat only", () => {
  setupChats("/agents/s2")
  leaderKey()
  press("KeyR")
  expect(screen.getByTestId("renaming").textContent).toBe("s2")
  act(() => closeRename())
})

test("{leader} r from the Message box opens the popup and keeps the draft", () => {
  setupChats("/agents/s1")
  const field = screen.getByLabelText("Message") as HTMLTextAreaElement
  field.focus()
  fireEvent.change(field, { target: { value: "half a thought" } })
  leaderKey(field)
  press("KeyR")
  expect(screen.getByTestId("renaming").textContent).toBe("s1")
  expect(field.value).toBe("half a thought")
  act(() => closeRename())
})

describe("on a draft page", () => {
  const actions = {
    remove: vi.fn(),
    rename: vi.fn(),
    isEmpty: () => true,
    focusPrompt: vi.fn(),
    openHistory: vi.fn(),
  }
  let unregister = () => {}
  beforeEach(() => {
    actions.remove.mockClear()
    actions.rename.mockClear()
    unregister = registerDraftPage(actions)
  })
  afterEach(() => unregister())

  test("{leader} r runs the page's rename and x its remove", () => {
    setupChats("/drafts/d1")
    leaderKey()
    expect(screen.getByRole("menuitem", { name: /Rename/ })).toBeTruthy()
    press("KeyR")
    expect(actions.rename).toHaveBeenCalledTimes(1)
    leaderKey()
    press("KeyX")
    expect(actions.remove).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId("renaming").textContent).toBe("none")
  })

  test("r and x do nothing on a page that is neither a chat nor a draft", () => {
    setupChats("/epics")
    leaderKey()
    press("KeyR")
    leaderKey()
    press("KeyX")
    expect(actions.rename).not.toHaveBeenCalled()
    expect(actions.remove).not.toHaveBeenCalled()
  })
})

test("{leader} r does nothing on a Queue agent page", () => {
  setupChats("/agents/q1", [session("q1", "gamma", { queue: true })])
  leaderKey()
  expect(screen.queryByRole("menuitem", { name: /Rename chat/ })).toBeNull()
  press("KeyR")
  expect(screen.getByTestId("renaming").textContent).toBe("none")
})

describe("Leader then h", () => {
  const design = session("s3", "gx", { design: true, ticket: "gx:ep/1" })
  const item = () => screen.queryByRole("menuitem", { name: /History/ })

  test.each([
    ["a Design draft page", "/drafts/d1", registerDraftPage],
    ["a Chat draft page", "/chat-drafts/c1", registerChatDraftPage],
  ])("opens History on %s", (_name, path, register) => {
    const openHistory = vi.fn()
    const unregister = (register as (a: unknown) => () => void)({
      remove: vi.fn(),
      rename: vi.fn(),
      isEmpty: () => true,
      focusPrompt: vi.fn(),
      focusBrief: vi.fn(),
      openHistory,
    })
    setupChats(path)
    leaderKey()
    expect(item()).toBeTruthy()
    press("KeyH")
    expect(openHistory).toHaveBeenCalledTimes(1)
    unregister()
  })

  test.each([
    ["a chat's page", "/agents/s1", session("s1", "gamma")],
    ["a Design's page", "/d/gx:ep", design],
  ])("opens the composer's History on %s", (_name, path, s) => {
    const open = vi.fn()
    const unregister = registerHistoryOpener(open)
    setupChats(path, [s])
    leaderKey()
    expect(item()).toBeTruthy()
    press("KeyH")
    expect(open).toHaveBeenCalledTimes(1)
    unregister()
  })

  test.each([
    ["with no opener registered", "/agents/s1", false],
    ["on a Design page with no opener", "/d/gx:ep", false],
    ["on a Queue agent", "/agents/s1", true],
    ["off the four pages", "/epics", true],
  ])("does nothing %s", (_name, path, registered) => {
    const open = vi.fn()
    const unregister = registered ? registerHistoryOpener(open) : () => {}
    const s =
      path === "/d/gx:ep" ? design : session("s1", "gamma", { queue: _name.includes("Queue") })
    setupChats(path, [s])
    leaderKey()
    expect(item()).toBeNull()
    press("KeyH")
    expect(open).not.toHaveBeenCalled()
    unregister()
  })

  test("a Design page does not offer r, m or x", () => {
    setupChats("/d/gx:ep", [design])
    leaderKey()
    for (const name of [/Rename/, /Switch model/, /Remove/]) {
      expect(screen.queryByRole("menuitem", { name })).toBeNull()
    }
  })
})
