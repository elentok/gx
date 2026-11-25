import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type {
  EpicDetail,
  SessionChatResponse,
  SessionInfo,
  SessionPromptRequest,
  SessionResult,
  Workspace,
} from "../generated/server"
import { appRoutes, type RouteFetchers } from "../routes"
import { renderThemed } from "../ui/render"
import { WriteError } from "../useWrite"
import { rowToken, stateToken } from "./open"

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const session = (id: string, over: Partial<SessionInfo> = {}): SessionInfo => ({
  id,
  name: id,
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: "2026-10-23T10:00:00.123456789Z",
  run_started_at: "2026-10-23T10:00:00.123456789Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
  ...over,
})

const detailOf = (over: Partial<EpicDetail> = {}): EpicDetail => ({
  address: "gx:a",
  kind: "map",
  summary: "",
  done: 0,
  total: 0,
  agent_time: 0,
  cost_total: 0,
  stats: {},
  rows: [],
  queueable: false,
  queued: false,
  steps: [
    { step: "chart", label: "Chart", state: "current" },
    { step: "done", label: "Done", state: "todo" },
  ],
  ...over,
})

const chat: SessionChatResponse = {
  conversation_id: "c",
  messages: [],
  questions: [],
  truncated: false,
} as unknown as SessionChatResponse
const ok: SessionResult = { session: session("new") }

function render(
  path: string,
  sessions: SessionInfo[],
  over: Partial<RouteFetchers> = {},
  detail = detailOf(),
  placed: { card?: object; workspaces?: Workspace[] } = {},
) {
  const state = {
    snapshot: { sessions, workspaces: placed.workspaces },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({
        epics: [
          {
            address: "gx:a",
            project: "gx",
            name: "a",
            kind: "map",
            state: "designing",
            done: 0,
            total: 0,
            queued: false,
            queueable: false,
            ...placed.card,
          },
        ],
        follow_ups: [],
      }),
      epic: async () => ({ kind: "ok", detail }),
      chat: async () => ({ kind: "ok", detail: chat }),
      ...over,
    }),
    { initialEntries: [path] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

test("opening the Design page with no session writes nothing and offers Start", async () => {
  const openDesign = vi.fn(async () => ok)
  const prompt = vi.fn()
  render("/d/gx:a", [], { openDesign, prompt })
  await screen.findByRole("button", { name: "Start design session" })
  expect(openDesign).not.toHaveBeenCalled()
  expect(prompt).not.toHaveBeenCalled()
})

test("a double click on Start opens once", async () => {
  const openDesign = vi.fn(() => new Promise<SessionResult>(() => {}))
  render("/d/gx:a", [], { openDesign })
  const start = await screen.findByRole("button", {
    name: "Start design session",
  })
  fireEvent.click(start)
  fireEvent.click(start)
  expect(openDesign).toHaveBeenCalledTimes(1)
  expect(openDesign).toHaveBeenCalledWith({ epic: "gx:a" })
})

test("a refused Start shows the message", async () => {
  render("/d/gx:a", [], {
    openDesign: async () => ({ refused: true, message: "no trunk checkout" }),
  })
  fireEvent.click(await screen.findByRole("button", { name: "Start design session" }))
  expect(await screen.findByText("no trunk checkout")).toBeTruthy()
})

test("Start is disabled while the write is pending, then a server error shows its text in a toast", async () => {
  let fail: (e: Error) => void = () => {}
  const openDesign = vi.fn(() => new Promise<SessionResult>((_, reject) => (fail = reject)))
  render("/d/gx:a", [], { openDesign })
  fireEvent.click(await screen.findByRole("button", { name: "Start design session" }))
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "Start design session" }) as HTMLButtonElement).disabled,
    ).toBe(true),
  )
  fail(new WriteError("trunk is dirty, commit first"))
  expect(await screen.findByText("trunk is dirty, commit first")).toBeTruthy()
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "Start design session" }) as HTMLButtonElement).disabled,
    ).toBe(false),
  )
})

test("the map epic page reads Start without a session and goes to the Design after it", async () => {
  const openDesign = vi.fn(async () => ok)
  const router = render("/e/gx:a", [], { openDesign })
  fireEvent.click(await screen.findByRole("button", { name: "Start design session" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/d/gx:a"))
  expect(openDesign).toHaveBeenCalledWith({ epic: "gx:a" })
})

test("the map epic page reads Open with a session and only navigates", async () => {
  const openDesign = vi.fn(async () => ok)
  const router = render("/e/gx:a", [session("s1", { status: { state: "exited" } })], { openDesign })
  fireEvent.click(await screen.findByRole("button", { name: "Open design session" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/d/gx:a"))
  expect(openDesign).not.toHaveBeenCalled()
})

test("a Done Design has no session button on its epic page", async () => {
  render("/e/gx:a", [], {}, detailOf({ steps: [{ step: "done", label: "Done", state: "done" }] }))
  await screen.findByText("No tickets yet")
  expect(screen.queryByRole("button", { name: /design session/ })).toBeNull()
})

const openMenu = async () =>
  fireEvent.click(await screen.findByRole("button", { name: "More actions" }))
const newSession = () => screen.findByRole("menuitem", { name: /New session/ })

const confirmDialog = () => screen.findByRole("alertdialog")
const confirmButton = (dialog: HTMLElement, name: RegExp) =>
  within(dialog).getByRole("button", { name })

test("New session opens the shared confirm, then opens new with the state token", async () => {
  const s = session("s1")
  const openDesign = vi.fn(async () => ok)
  const confirm = vi.spyOn(window, "confirm")
  render("/d/gx:a", [s], { openDesign })
  await openMenu()
  fireEvent.click(await newSession())
  const dialog = await confirmDialog()
  expect(openDesign).not.toHaveBeenCalled()
  expect(document.activeElement).toBe(confirmButton(dialog, /^Cancel/))
  fireEvent.click(confirmButton(dialog, /^New session/))
  await waitFor(() =>
    expect(openDesign).toHaveBeenCalledWith({
      epic: "gx:a",
      new: true,
      state: stateToken(s),
    }),
  )
  expect(confirm).not.toHaveBeenCalled()
})

test("Cancel, Esc and n send nothing from the New session confirm", async () => {
  const openDesign = vi.fn(async () => ok)
  render("/d/gx:a", [session("s1")], { openDesign })
  const dismiss = [
    async (dialog: HTMLElement) => fireEvent.click(confirmButton(dialog, /^Cancel/)),
    async (dialog: HTMLElement) => fireEvent.keyDown(dialog, { key: "Escape" }),
    async () => fireEvent.keyDown(document.activeElement ?? document.body, { key: "n" }),
  ]
  for (const how of dismiss) {
    await openMenu()
    fireEvent.click(await newSession())
    await how(await confirmDialog())
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
  }
  expect(openDesign).not.toHaveBeenCalled()
})

test("New session on an exited session asks nothing and sends no state token", async () => {
  const openDesign = vi.fn(async () => ok)
  render("/d/gx:a", [session("s1", { status: { state: "exited" } })], { openDesign })
  await openMenu()
  fireEvent.click(await newSession())
  await waitFor(() => expect(openDesign).toHaveBeenCalledWith({ epic: "gx:a", new: true }))
  expect(screen.queryByRole("alertdialog")).toBeNull()
})

test("Stop on the shown live session confirms every time and posts the row token", async () => {
  const s = session("s1")
  const stop = vi.fn(async () => ({}) as SessionResult)
  render("/d/gx:a", [s], { stop })
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Stop" }))
  fireEvent.click(confirmButton(await confirmDialog(), /^Cancel/))
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
  expect(stop).not.toHaveBeenCalled()
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Stop" }))
  fireEvent.click(confirmButton(await confirmDialog(), /^Stop/))
  await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "s1", state: rowToken(s) }))
})

test("an exited shown session and a Design with no session offer no Stop", async () => {
  render("/d/gx:a", [session("s1", { status: { state: "exited" } })])
  await openMenu()
  await newSession()
  expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull()
  cleanup()
  render("/d/gx:a", [])
  await openMenu()
  await screen.findByRole("menuitem", { name: "Copy ID" })
  expect(screen.queryByRole("menuitem", { name: "Stop" })).toBeNull()
})

test("the row token is the id, the run start as held, and live or exited", () => {
  const s = session("s1", { run_started_at: "2026-10-23T09:00:00.5Z" })
  expect(rowToken(s)).toBe("s1:2026-10-23T09:00:00.5Z:live")
  expect(rowToken({ ...s, status: { state: "working" } })).toBe(rowToken(s))
  expect(rowToken({ ...s, status: { state: "exited" } })).toBe("s1:2026-10-23T09:00:00.5Z:exited")
})

test("the state token has the server's shape", () => {
  expect(stateToken(session("s1"))).toBe("s1:idle:1792749600123456789")
})

test("the menu lists Live sessions only with 2 or more, and has none with one", async () => {
  render("/d/gx:a", [session("only")])
  await openMenu()
  await newSession()
  expect(screen.queryByText(/Live sessions/)).toBeNull()
})

const spaces: Workspace[] = [
  { name: "Default", default: true, members: 1 },
  { name: "Work", members: 1 },
]

test("the menu reads Live sessions first, then the registry groups, with Copy ID once", async () => {
  render("/d/gx:a", [session("a"), session("b")], {}, detailOf({ deletable: true }), {
    card: { addresses: ["gx:a"], workspace: "Work" },
    workspaces: spaces,
  })
  await openMenu()
  await newSession()
  const live = screen.getByText(/Live sessions/)
  const items = screen.getAllByRole("menuitem")
  const labels = items.map((i) => i.textContent ?? "")
  const open = labels.indexOf("Open design epic")
  // Two live sessions: Delete design is not offered.
  const registry = labels.slice(open)
  expect(registry.map((l) => l.replace(/›$/, ""))).toEqual([
    "Open design epic",
    "Move to workspace",
    "New session",
    "Hold",
    "Stop",
    "Archive",
    "Copy ID",
  ])
  expect(live.compareDocumentPosition(items[open]!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  // A rule separates every registry group.
  expect(screen.getAllByRole("separator").length).toBeGreaterThanOrEqual(4)
  expect(labels.filter((l) => /^Copy/.test(l))).toEqual(["Copy ID"])
})

test("a deletable Design with no session ends its menu with Delete design in the Danger group", async () => {
  render("/d/gx:a", [], {}, detailOf({ deletable: true }), {
    card: { addresses: ["gx:a"], workspace: "Work" },
    workspaces: spaces,
  })
  await openMenu()
  await screen.findByRole("menuitem", { name: "Copy ID" })
  const labels = screen.getAllByRole("menuitem").map((i) => i.textContent?.replace(/›$/, ""))
  expect(labels).toEqual([
    "Open design epic",
    "Move to workspace",
    "Hold",
    "Archive",
    "Copy ID",
    "Delete design",
  ])
})

test("a held Design offers Take off hold and says On hold in its subtitle", async () => {
  render("/d/gx:a", [session("a")], {}, detailOf(), {
    card: { addresses: ["gx:a"], workspace: "Work", section: "on-hold" },
    workspaces: spaces,
  })
  await openMenu()
  expect(await screen.findByRole("menuitem", { name: "Take off hold" })).toBeTruthy()
  expect(screen.queryByRole("menuitem", { name: "Hold" })).toBeNull()
  expect(screen.getByText(/On hold/)).toBeTruthy()
})

test("the Live sessions list switches the page and stops another session with a token", async () => {
  // The older run flipped state last; the newer run start still wins.
  const older = session("old", {
    run_started_at: "2026-10-23T09:00:00Z",
    status_changed_at: "2026-10-23T11:00:00Z",
  })
  const newer = session("new", {
    run_started_at: "2026-10-23T10:00:00Z",
    status_changed_at: "2026-10-23T10:00:00Z",
  })
  const stop = vi.fn(async () => ({}) as SessionResult)
  const chatFor = vi.fn(async () => ({ kind: "ok" as const, detail: chat }))
  render("/d/gx:a", [older, newer], { stop, chat: chatFor })
  await waitFor(() => expect(chatFor).toHaveBeenCalledWith("new"))
  await openMenu()
  expect(await screen.findByText("Live sessions · 2")).toBeTruthy()
  expect(screen.getByRole("menuitem", { name: "new ✓" }).getAttribute("aria-current")).toBe("true")
  expect(screen.queryByRole("menuitem", { name: "Stop new" })).toBeNull()
  // The registry's Stop (exact name) stops the shown session; the other one has its own row.
  expect(screen.getByRole("menuitem", { name: "Stop" })).toBeTruthy()
  expect(screen.getByRole("menuitem", { name: "Stop old" })).toBeTruthy()
  fireEvent.click(screen.getByRole("menuitem", { name: "old" }))
  await waitFor(() => expect(chatFor).toHaveBeenCalledWith("old"))
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Stop new" }))
  // The menu closes, and the shared confirm names that session.
  const dialog = await confirmDialog()
  expect(within(dialog).getAllByText(/Stop new/).length).toBeGreaterThan(0)
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
  expect(stop).not.toHaveBeenCalled()
  fireEvent.click(confirmButton(dialog, /^Stop/))
  await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "new", state: rowToken(newer) }))
})

test("arrow keys reach another session's Stop item; Enter closes the menu and opens the confirm", async () => {
  const shown = session("new", {
    run_started_at: "2026-10-23T10:00:00Z",
    status_changed_at: "2026-10-23T10:00:00Z",
  })
  const other = session("old", {
    run_started_at: "2026-10-23T09:00:00Z",
    status_changed_at: "2026-10-23T09:00:00Z",
  })
  const stop = vi.fn(async () => ({}) as SessionResult)
  render("/d/gx:a", [shown, other], { stop })
  await openMenu()
  const menu = await screen.findByRole("menu")
  const stopItem = await screen.findByRole("menuitem", { name: "Stop old" })
  // Zag keeps focus on the menu and highlights one item at a time: the live sessions come first.
  const sessionItem = screen.getByRole("menuitem", { name: "old" })
  const arrowTo = async (target: HTMLElement) => {
    fireEvent.keyDown(menu, { key: "ArrowDown" })
    await waitFor(() => expect(target.hasAttribute("data-highlighted")).toBe(true))
  }
  await arrowTo(screen.getByRole("menuitem", { name: "new ✓" }))
  await arrowTo(sessionItem)
  await arrowTo(stopItem)
  fireEvent.keyDown(menu, { key: "Enter" })
  const dialog = await confirmDialog()
  expect(stop).not.toHaveBeenCalled()
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
  fireEvent.click(confirmButton(dialog, /^Stop/))
  await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "old", state: rowToken(other) }))
})

const phone = () =>
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
  }))

test("on a phone the ⋯ menu is an anchored popover with the desktop items", async () => {
  phone()
  try {
    const s = session("s1")
    const openDesign = vi.fn(async () => ok)
    const stop = vi.fn(async () => ({}) as SessionResult)
    const confirm = vi.spyOn(window, "confirm")
    const other = session("s2", {
      run_started_at: "2026-10-23T09:00:00Z",
      status_changed_at: "2026-10-23T09:00:00Z",
    })
    render("/d/gx:a", [s, other], { openDesign, stop })
    await openMenu()
    const menu = await screen.findByRole("menu")
    expect(screen.getByText("Live sessions · 2")).toBeTruthy()
    fireEvent.click(screen.getByRole("menuitem", { name: "Stop s2" }))
    fireEvent.click(confirmButton(await confirmDialog(), /^Stop/))
    await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "s2", state: rowToken(other) }))
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
    await openMenu()
    fireEvent.click(await newSession())
    fireEvent.click(confirmButton(await confirmDialog(), /^New session/))
    await waitFor(() => expect(openDesign).toHaveBeenCalled())
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
    expect(menu.isConnected).toBe(false)
    expect(confirm).not.toHaveBeenCalled()
  } finally {
    vi.unstubAllGlobals()
  }
})

test("on a phone a press on the backdrop closes the sheet", async () => {
  phone()
  try {
    render("/d/gx:a", [session("s1")])
    await openMenu()
    const menu = await screen.findByRole("menu")
    fireEvent.click(menu.parentElement!)
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull())
  } finally {
    vi.unstubAllGlobals()
  }
})

test("on a phone Delete design is in the sheet while it can run", async () => {
  phone()
  try {
    render("/d/gx:a", [], {}, detailOf({ deletable: true }))
    await openMenu()
    expect(await screen.findByRole("menuitem", { name: "Delete design" })).toBeTruthy()
  } finally {
    vi.unstubAllGlobals()
  }
})

test("a send goes to the session on screen", async () => {
  // The older run flipped state last; the newer run start still wins.
  const older = session("old", {
    run_started_at: "2026-10-23T09:00:00Z",
    status_changed_at: "2026-10-23T11:00:00Z",
  })
  const newer = session("new", {
    run_started_at: "2026-10-23T10:00:00Z",
    status_changed_at: "2026-10-23T10:00:00Z",
  })
  const prompt = vi.fn(async (_body: SessionPromptRequest) => ({}) as SessionResult)
  const withContinue = {
    ...chat,
    next: { action: { action: ["/clear", "/gx-design a"] } },
  } as SessionChatResponse
  render("/d/gx:a", [older, newer], {
    prompt,
    chat: async () => ({ kind: "ok", detail: withContinue }),
  })
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: "old" }))
  fireEvent.click(await screen.findByRole("button", { name: /Continue/ }))
  await waitFor(() => expect(prompt).toHaveBeenCalled())
  expect(prompt.mock.calls[0][0]).toMatchObject({ id: "old", continue: true })
})
