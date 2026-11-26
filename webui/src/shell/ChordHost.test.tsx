import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider, useLocation } from "react-router"
import { closeConfirm } from "../actions/confirmRequest"
import { ConfirmHost } from "./ConfirmHost"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { Modal } from "../ui/modal/Modal"
import { renderThemed } from "../ui/render"
import { ChordHost } from "./ChordHost"
import { installFakeNavigation } from "./fakeNavigation.testutil"
import { getBigPrompt, setBigPrompt } from "../ui/expandable-text/bigPromptStore"
import { chordTable, type Chord } from "./chords"
import { registerHistoryOpener } from "../history/historyOpener"
import { registerChatDraftPage, registerDraftPage } from "./draftPageActions"
import { WorkspacePickerHost } from "./WorkspacePickerHost"
import { closeRename, useRenameRequest } from "./renameOpen"
import type { FetchEpicList } from "../epics/list"
import { rowNavId } from "../workspaces/sidebarModel"

beforeEach(() => {
  vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
})
afterEach(() => {
  cleanup()
  closeConfirm()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const chord = (key: string, run: () => void): Chord => ({
  key,
  label: () => `Do ${key}`,
  run,
  available: () => true,
})

const emptyList: FetchEpicList = async () => ({ epics: [], follow_ups: [] })

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
            <ChordHost chords={chords} fetchEpicList={emptyList} active="Default" />
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
  const table = () => chordTable({ actions: [], history: () => {} })
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

function setupChats(
  path: string,
  sessions = [session("s1", "gamma"), session("s2", "gamma")],
  snapshot: Record<string, unknown> = {},
  extra: React.ReactNode = null,
) {
  const state = {
    snapshot: { leader_key: "ctrl+e", sessions, ...snapshot },
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
            <PathProbe />
            <ConfirmHost />
            <WorkspacePickerHost />
            {extra}
            <ChordHost fetchEpicList={emptyList} active="Default" />
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
  return router
}

/** Answers each request with the body of the first route whose text is in the URL, else `{}`. */
function stubApi(routes: Record<string, unknown>) {
  const fetchMock = vi.fn(async (url: RequestInfo | URL, _init?: RequestInit) => {
    const hit = Object.entries(routes).find(([route]) => String(url).includes(route))
    return new Response(JSON.stringify(hit?.[1] ?? {}), { status: 200 })
  })
  vi.stubGlobal("fetch", fetchMock)
  return fetchMock
}

/** The keys of the menu rows, in the order shown. */
const menuKeys = () =>
  screen
    .getAllByRole("menuitem")
    .map((i) => i.querySelector("b")?.textContent ?? "")
    .filter((k) => !["c", "C", "d", "s", "b", "h"].includes(k))

function PathProbe() {
  return <div data-testid="path">{useLocation().pathname}</div>
}

test("{leader} x on a live chat opens the shared confirm as a modal, then lands on /agents", async () => {
  const fetchMock = vi.fn(
    async () =>
      new Response(JSON.stringify({ results: [{ target: "s2", status: "done" }] }), {
        status: 200,
      }),
  )
  vi.stubGlobal("fetch", fetchMock)
  setupChats("/agents/s2")
  leaderKey()
  press("KeyX")
  const dialog = await screen.findByRole("dialog", { name: /Remove/ })
  expect(dialog.textContent).toContain("Stop and remove")
  expect(fetchMock).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: /^Remove/ }))
  await waitFor(() => expect(screen.getByTestId("path").textContent).toBe("/agents"))
  expect(fetchMock).toHaveBeenCalled()
  vi.unstubAllGlobals()
})

describe("x on a chat opens a modal confirm", () => {
  afterEach(() => vi.unstubAllGlobals())

  test("with Cancel focused; n cancels and posts nothing", async () => {
    const fetchMock = stubApi({})
    setupChats("/agents/s2")
    leaderKey()
    press("KeyX")
    const dialog = await screen.findByRole("dialog", { name: /Remove/ })
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: /^Cancel/ })),
    )
    fireEvent.keyDown(window, { key: "n", code: "KeyN" })
    await waitFor(() => expect(dialog.isConnected).toBe(false))
    expect(fetchMock).not.toHaveBeenCalled()
    expect(screen.getByTestId("path").textContent).toBe("/agents/s2")
  })

  test("y confirms and the chat's page is left", async () => {
    stubApi({ "/v1/sessions": { results: [{ target: "s2", status: "done" }] } })
    setupChats("/agents/s2")
    leaderKey()
    press("KeyX")
    await screen.findByRole("dialog", { name: /Remove/ })
    fireEvent.keyDown(window, { key: "y", code: "KeyY" })
    await waitFor(() => expect(screen.getByTestId("path").textContent).toBe("/agents"))
  })
})

describe("chat draft page", () => {
  let empty = true
  const actions = {
    isEmpty: () => empty,
    focusBrief: vi.fn(),
    deleteNow: vi.fn(),
    rename: vi.fn(),
  }
  let unregister = () => {}
  beforeEach(() => {
    empty = true
    actions.deleteNow.mockClear()
    actions.rename.mockClear()
    unregister = registerChatDraftPage(actions)
  })
  afterEach(() => unregister())

  test("{leader} r runs the page's rename and x on an empty draft deletes it at once", async () => {
    setupChats("/chat-drafts/c1")
    leaderKey()
    expect(screen.getByRole("menuitem", { name: /Remove/ })).toBeTruthy()
    press("KeyR")
    expect(actions.rename).toHaveBeenCalledTimes(1)
    leaderKey()
    press("KeyX")
    await waitFor(() => expect(actions.deleteNow).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole("dialog", { name: /Remove/ })).toBeNull()
  })

  test("{leader} x on a draft with content opens one confirm, and Remove deletes it", async () => {
    empty = false
    setupChats("/chat-drafts/c1")
    leaderKey()
    press("KeyX")
    await screen.findByRole("dialog", { name: /Remove/ })
    expect(actions.deleteNow).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole("button", { name: /^Remove/ }))
    await waitFor(() => expect(actions.deleteNow).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByTestId("path").textContent).toBe("/epics"))
  })

  test("{leader} y does nothing: a draft has no ID", () => {
    setupChats("/chat-drafts/c1")
    leaderKey()
    expect(screen.queryByRole("menuitem", { name: /Copy ID/ })).toBeNull()
  })
})

describe("epic and Design pages", () => {
  const card = { address: "gx:ep", addresses: ["gx:ep"], name: "ep", workspace: "Default" }
  const workspaces = [
    { name: "Default", default: true, members: 1 },
    { name: "Work", members: 0 },
  ]
  afterEach(() => vi.unstubAllGlobals())

  // The page's card arrives with the epic list; a chord box opened earlier would not show its chords.
  async function openAfterList(fetchMock: ReturnType<typeof stubApi>) {
    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([u]) => String(u).endsWith("/v1/epics"))).toBe(true),
    )
    await act(async () => {})
    leaderKey()
  }

  test("a on an epic runs the Archive dry run, then opens the confirm as a modal", async () => {
    const fetchMock = stubApi({
      "/v1/epics/archive": { results: [{ target: "gx:ep", status: "done" }] },
      "/v1/epics": { epics: [card] },
    })
    setupChats("/e/gx:ep", [], { workspaces })
    await openAfterList(fetchMock)
    expect(screen.getByRole("menuitem", { name: /Archive/ })).toBeTruthy()
    press("KeyA")
    await screen.findByRole("dialog", { name: /Archive/ })
    const archive = fetchMock.mock.calls.filter(([u]) => String(u).includes("/v1/epics/archive"))
    expect(archive.map(([, init]) => JSON.parse(String(init?.body)))).toEqual([
      { targets: ["gx:ep"], dry_run: true },
    ])
  })

  test("w opens a workspace picker with the current one marked; a pick posts the move", async () => {
    const fetchMock = stubApi({
      "/v1/workspaces/move": { results: [{ target: "gx:ep", status: "done" }] },
      "/v1/epics": { epics: [card] },
    })
    setupChats("/e/gx:ep", [], { workspaces })
    await openAfterList(fetchMock)
    press("KeyW")
    const dialog = await screen.findByRole("dialog", { name: "Move to workspace" })
    expect(screen.getByRole("button", { name: "Default (current)" })).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Work" }))
    await waitFor(() => expect(dialog.isConnected).toBe(false))
    const move = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/workspaces/move"))
    expect(JSON.parse(String(move?.[1]?.body))).toEqual({ name: "Work", targets: ["gx:ep"] })
  })

  test("an epic outside every workspace offers y only", async () => {
    const fetchMock = stubApi({ "/v1/epics": { epics: [{ ...card, workspace: "" }] } })
    setupChats("/e/gx:ep", [], { workspaces })
    await openAfterList(fetchMock)
    expect(menuKeys()).toEqual(["y"])
  })

  test("a Design page offers w, y and a once its card is in the list", async () => {
    const design = session("s3", "gx", { design: true, ticket: "gx:ep/1" })
    const fetchMock = stubApi({ "/v1/epics": { epics: [card] } })
    setupChats("/d/gx:ep", [design], { workspaces })
    await openAfterList(fetchMock)
    expect(menuKeys()).toEqual(["w", "a", "y"])
    for (const name of [/Rename/, /Switch model/, /Remove/]) {
      expect(screen.queryByRole("menuitem", { name })).toBeNull()
    }
  })

  test("an archived epic offers Unarchive on a and Copy ID on y", () => {
    setupChats("/archive/gx:ep")
    leaderKey()
    expect(screen.getByRole("menuitem", { name: /Unarchive/ })).toBeTruthy()
    expect(menuKeys()).toEqual(["a", "y"])
  })

  test("a ticket page offers y only", () => {
    setupChats("/t/gx:ep/01")
    leaderKey()
    expect(menuKeys()).toEqual(["y"])
  })
})

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
    deleteNow: vi.fn(),
    rename: vi.fn(),
    isEmpty: () => true,
    focusPrompt: vi.fn(),
  }
  let unregister = () => {}
  beforeEach(() => {
    actions.deleteNow.mockClear()
    actions.rename.mockClear()
    unregister = registerDraftPage(actions)
  })
  afterEach(() => unregister())

  test("{leader} r runs the page's rename and x deletes the empty draft", async () => {
    setupChats("/drafts/d1")
    leaderKey()
    expect(screen.getByRole("menuitem", { name: /Rename/ })).toBeTruthy()
    press("KeyR")
    expect(actions.rename).toHaveBeenCalledTimes(1)
    leaderKey()
    press("KeyX")
    await waitFor(() => expect(actions.deleteNow).toHaveBeenCalledTimes(1))
    expect(screen.getByTestId("renaming").textContent).toBe("none")
  })

  test("r and x do nothing on a page that is neither a chat nor a draft", () => {
    setupChats("/epics")
    leaderKey()
    press("KeyR")
    leaderKey()
    press("KeyX")
    expect(actions.rename).not.toHaveBeenCalled()
    expect(actions.deleteNow).not.toHaveBeenCalled()
  })
})

test("{leader} r does nothing on a Queue agent page", () => {
  setupChats("/agents/q1", [session("q1", "gamma", { queue: true })])
  leaderKey()
  expect(screen.queryByRole("menuitem", { name: /Rename chat/ })).toBeNull()
  press("KeyR")
  expect(screen.getByTestId("renaming").textContent).toBe("none")
})

describe("Leader then , and .", () => {
  afterEach(() => vi.unstubAllGlobals())

  const path = () => screen.getByTestId("path").textContent
  const row = (name: string) => screen.queryByRole("menuitem", { name: new RegExp(name) })

  /**
   * Three entries, standing on the middle one, so both directions have somewhere to go. The spy
   * starts after that: a move lands late, so "stays" is asserted on the request for it.
   */
  async function onMiddleEntry() {
    const router = setupChats("/epics")
    await act(() => router.navigate("/agents"))
    await act(() => router.navigate("/archive"))
    await act(() => router.navigate(-1))
    expect(path()).toBe("/agents")
    return vi.spyOn(router, "navigate")
  }

  test.each([
    ["Comma", ",", "/epics"],
    ["Comma", "ת", "/epics"],
    ["Period", ".", "/archive"],
  ])("code %s with key %s lands on %s", async (code, key, target) => {
    installFakeNavigation({ canGoBack: true, canGoForward: true })
    await onMiddleEntry()
    leaderKey()
    press(code, { key })
    await waitFor(() => expect(path()).toBe(target))
    expect(box()).toBeNull()
  })

  test.each(
    ["Comma", "Period"].flatMap((code) =>
      ["shiftKey", "metaKey", "ctrlKey", "altKey"].map((mod) => [mod, code]),
    ),
  )("%s with %s closes the box and stays", async (mod, code) => {
    installFakeNavigation({ canGoBack: true, canGoForward: true })
    const navigate = await onMiddleEntry()
    leaderKey()
    press(code, { [mod]: true })
    expect(box()).toBeNull()
    expect(navigate).not.toHaveBeenCalled()
  })

  test("the open box follows currententrychange and a hidden row's key stays", async () => {
    const history = installFakeNavigation({ canGoBack: true })
    const navigate = await onMiddleEntry()
    leaderKey()
    expect([row("Back") !== null, row("Forward") !== null]).toEqual([true, false])
    act(() => history.set({ canGoBack: false, canGoForward: true }))
    expect([row("Back") !== null, row("Forward") !== null]).toEqual([false, true])
    press("Comma")
    expect(box()).toBeNull()
    expect(navigate).not.toHaveBeenCalled()
  })

  test("without the Navigation API there are no rows and the key only closes the box", async () => {
    const navigate = await onMiddleEntry()
    leaderKey()
    expect([row("Back"), row("Forward")]).toEqual([null, null])
    press("Comma")
    expect(box()).toBeNull()
    expect(navigate).not.toHaveBeenCalled()
  })
})

describe("Leader then j and k", () => {
  const scrollIntoView = Element.prototype.scrollIntoView
  afterEach(() => {
    Element.prototype.scrollIntoView = scrollIntoView
  })

  const ids = ["s1", "s2"]
  const path = () => screen.getByTestId("path").textContent
  const row = (name: string) => screen.queryByRole("menuitem", { name: new RegExp(name) })
  const nextRow = () => screen.findByRole("menuitem", { name: /Next item/ })

  const mount = (at: string) =>
    setupChats(
      at,
      ids.map((id) => session(id, "gamma")),
      { placements: ids.map((id) => ({ session: id, kind: "chat", workspace: "Default" })) },
      <ul data-nav-list="">
        {ids.map((id) => (
          <li key={id}>
            <a href={`#/agents/${id}`} data-nav={rowNavId(id)}>
              {id}
            </a>
          </li>
        ))}
      </ul>,
    )

  /** Opens the box and waits for the row, which needs the epic list. */
  async function step(code: "KeyJ" | "KeyK", name: string) {
    leaderKey()
    await screen.findByRole("menuitem", { name: new RegExp(name) })
    press(code)
  }

  test("opens the next and previous row as new entries, with no state and no focus move", async () => {
    const router = mount("/agents/s1")
    const field = screen.getByLabelText("Message")
    field.focus()

    await step("KeyJ", "Next item")
    await waitFor(() => expect(path()).toBe("/agents/s2"))
    expect([router.state.historyAction, router.state.location.state]).toEqual(["PUSH", null])
    expect(document.activeElement).toBe(field)

    await step("KeyK", "Previous item")
    await waitFor(() => expect(path()).toBe("/agents/s1"))
    expect([router.state.historyAction, router.state.location.state]).toEqual(["PUSH", null])
  })

  test.each(
    ["KeyJ", "KeyK"].flatMap((code) =>
      ["shiftKey", "metaKey", "ctrlKey", "altKey"].map((mod) => [mod, code]),
    ),
  )("%s with %s closes the box and stays", async (mod, code) => {
    // From /settings both keys have a target.
    const router = mount("/settings")
    leaderKey()
    await nextRow()
    const navigate = vi.spyOn(router, "navigate")
    press(code, { [mod]: true })
    expect(box()).toBeNull()
    expect(navigate).not.toHaveBeenCalled()
  })

  test("a row with no target is hidden, its key stays, and the open box follows the sidebar", async () => {
    const router = mount("/agents/s1")
    leaderKey()
    await nextRow()
    expect(row("Previous item")).toBeNull()
    const navigate = vi.spyOn(router, "navigate")
    press("KeyK")
    expect(box()).toBeNull()
    expect(navigate).not.toHaveBeenCalled()

    leaderKey()
    expect(row("Next item")).toBeTruthy()
    act(() => screen.getByText("s2").remove())
    await waitFor(() => expect(row("Next item")).toBeNull())
  })

  test("without the Navigation API j still opens a row and there is no Back or Forward", async () => {
    mount("/settings")
    leaderKey()
    await nextRow()
    expect([row("Back"), row("Forward")]).toEqual([null, null])
    press("KeyJ")
    await waitFor(() => expect(path()).toBe("/agents/s1"))
  })

  test("on a page with no row, j continues from the last row, also after a remount", async () => {
    const router = mount("/agents/s1")
    // Stored with no chord run.
    await waitFor(() => expect(sessionStorage.getItem("gx.sidebar.last-row")).toBe(rowNavId("s1")))
    await act(() => router.navigate("/settings"))
    await step("KeyJ", "Next item")
    await waitFor(() => expect(path()).toBe("/agents/s2"))

    await act(() => router.navigate("/agents/s1"))
    await waitFor(() => expect(sessionStorage.getItem("gx.sidebar.last-row")).toBe(rowNavId("s1")))
    cleanup()
    mount("/settings")
    await step("KeyJ", "Next item")
    await waitFor(() => expect(path()).toBe("/agents/s2"))
  })

  test("with blocked storage the last row still works", async () => {
    const blocked = () => {
      throw new Error("blocked")
    }
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(blocked)
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(blocked)
    const router = mount("/agents/s1")
    leaderKey()
    await nextRow()
    press("Escape")
    await act(() => router.navigate("/settings"))
    await step("KeyJ", "Next item")
    await waitFor(() => expect(path()).toBe("/agents/s2"))
  })

  test("a step scrolls the new row into view", async () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    mount("/agents/s1")
    await step("KeyJ", "Next item")
    expect(scroll.mock.contexts).toEqual([screen.getByText("s2")])
    expect(scroll).toHaveBeenCalledWith({ block: "nearest" })
  })
})

describe("Leader then h", () => {
  const design = session("s3", "gx", { design: true, ticket: "gx:ep/1" })
  const item = () => screen.queryByRole("menuitem", { name: /History/ })

  test.each([
    ["a Design draft page", "/drafts/d1", []],
    ["a Chat draft page", "/chat-drafts/c1", []],
    ["a chat's page", "/agents/s1", [session("s1", "gamma")]],
    ["a Design's page", "/d/gx:ep", [design]],
  ])("opens the registered History on %s", (_name, path, sessions) => {
    const open = vi.fn()
    const unregister = registerHistoryOpener(open)
    setupChats(path, sessions)
    leaderKey()
    expect(item()).toBeTruthy()
    press("KeyH")
    expect(open).toHaveBeenCalledTimes(1)
    unregister()
  })

  test.each([
    ["a chat's page", "/agents/s1"],
    ["a Design page", "/d/gx:ep"],
    ["a draft page", "/drafts/d1"],
  ])("does nothing on %s with no opener registered", (_name, path) => {
    setupChats(path, [design, session("s1", "gamma")])
    leaderKey()
    expect(item()).toBeNull()
    press("KeyH")
  })

  test("a Design page does not offer r, m or x", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}")),
    )
    setupChats("/d/gx:ep", [design])
    leaderKey()
    for (const name of [/Rename/, /Switch model/, /Remove/]) {
      expect(screen.queryByRole("menuitem", { name })).toBeNull()
    }
    vi.unstubAllGlobals()
  })
})
