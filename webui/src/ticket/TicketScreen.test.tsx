import { cleanup, fireEvent, screen, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import canned from "../canned-detail.json"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { FetchScreen } from "../design/screen"
import type { PendingRow, TicketDetail } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { FetchDetail } from "./detail"

afterEach(cleanup)

const state = {
  connected: true,
  loadedOnce: true,
  tick: 0,
  snapshot: { tickets: [], epics: [] },
} as unknown as ConnectionState
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection

const base: TicketDetail = { ...(canned as TicketDetail), runs: [], related: [], epic: "gx:demo" }

async function renderTicket(
  detail: TicketDetail,
  ticketScreen?: FetchScreen,
  pending: PendingRow[] = [],
) {
  state.snapshot = { tickets: [], epics: [], pending } as unknown as ConnectionState["snapshot"]
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail })
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail, ticketScreen }), {
    initialEntries: ["/t/gx:demo/12"],
  })
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  // While the detail loads the h1 shows the address, so wait for the loaded title.
  await screen.findByRole("heading", { level: 1, name: detail.title.trim() })
}

test("a ticket with a pty agent has a Screen button that opens the screen by ticket address", async () => {
  const fetchRows = vi.fn<FetchScreen>(async () => ({
    kind: "ok",
    detail: { rows: ["agent row"] },
  }))
  await renderTicket({ ...base, has_screen: true }, fetchRows)

  fireEvent.click(screen.getByRole("button", { name: "Screen" }))
  const dialog = await screen.findByRole("dialog", { name: "Screen" })
  await within(dialog).findByText(/agent row/)
  expect(fetchRows).toHaveBeenCalledWith("gx:demo/12")
})

test("a ticket without a pty agent has no Screen button", async () => {
  await renderTicket({ ...base, has_screen: undefined })

  expect(screen.queryByRole("button", { name: "Screen" })).toBeNull()
})

test("the ticket header shows the check-lock waiting label as a status label, and nothing without it", async () => {
  const claimed = { ...base, status: "claimed" }
  await renderTicket({ ...claimed, waiting_label: "waiting for the check lock, held by run-1" })
  expect(document.querySelector("[data-waiting-label]")?.textContent).toBe(
    "waiting for the check lock, held by run-1",
  )
  expect(screen.queryByText(/Paused/)).toBeNull()
  cleanup()

  await renderTicket(claimed)
  expect(document.querySelector("[data-waiting-label]")).toBeNull()
})

// Replaces the ticket-page half of e2e flow 148 (Long-running badge).
test("the ticket header shows the Long-running badge only for a marked ticket", async () => {
  await renderTicket({ ...base, long_ticket_turns: 120, long_ticket_threshold: 100 })
  expect((await screen.findByText("Long-running")).getAttribute("title")).toBe(
    "120 assistant turns (threshold 100)",
  )
  cleanup()

  await renderTicket(base)
  expect(screen.queryByText("Long-running")).toBeNull()
})

test("the ticket page shows the launch step in the verdict slot, and the cap wait as Waiting for a slot", async () => {
  const claimed = {
    ...base,
    status: "claimed",
    stats: {
      ...base.stats,
      launch_step: { label: "installing dependencies", position: 2, total: 4 },
    },
  }
  await renderTicket(claimed, undefined, [
    { address: "gx:demo/12", agent: "", verdict: "concurrency cap", reason: "epic cap 3/2" },
  ])
  expect(document.querySelectorAll("[data-verdict]")).toHaveLength(1)
  expect(screen.getByText("Launching: installing dependencies (2/4)")).toBeTruthy()
  cleanup()

  await renderTicket(base, undefined, [
    { address: "gx:demo/12", agent: "", verdict: "concurrency cap", reason: "epic cap 3/2" },
  ])
  expect(screen.getByText("Waiting for a slot: epic cap 3/2")).toBeTruthy()
})

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()

test("a launching ticket shows the claimed_at timer beside the Launching label, and the timer stays once the run is working", async () => {
  const stats = { ...base.stats, elapsed: 0, claimed_at: minutesAgo(5) }
  await renderTicket({
    ...base,
    status: "claimed",
    stats: { ...stats, launch_step: { label: "starting agent", position: 3, total: 4 } },
  })
  expect(screen.getByText("Launching: starting agent (3/4)")).toBeTruthy()
  expect(screen.getByText("5m")).toBeTruthy()
  cleanup()

  // Working: the step is gone, the label with it, and the same claimed_at clock keeps counting.
  await renderTicket({ ...base, status: "claimed", stats })
  expect(screen.queryByText(/Launching/)).toBeNull()
  expect(screen.getByText("5m")).toBeTruthy()
})

test("a launch step change re-renders the ticket page with no reload", async () => {
  const listeners: ((a?: string) => void)[] = []
  const evConn = {
    ...conn,
    onEvent: (l: (a?: string) => void) => {
      listeners.push(l)
      return () => {}
    },
  } as unknown as Connection
  const stepped = (position: number, label: string): TicketDetail => ({
    ...base,
    status: "claimed",
    stats: { ...base.stats, launch_step: { label, position, total: 4 } },
  })
  let current = stepped(2, "installing dependencies")
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail: current })
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail }), {
    initialEntries: ["/t/gx:demo/12"],
  })
  renderThemed(
    <ConnectionContext.Provider value={evConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  await screen.findByText("Launching: installing dependencies (2/4)")

  for (const [position, label] of [
    [3, "starting agent"],
    [4, "waiting for the agent"],
  ] as const) {
    current = stepped(position, label)
    listeners.forEach((l) => {
      l("gx:demo/12")
    })
    await screen.findByText(`Launching: ${label} (${position}/4)`)
  }
  expect(screen.queryByText(/installing dependencies/)).toBeNull()
})

test("a ticket page refetches when the event stream reopens, since events sent while it was down never reached it", async () => {
  const subscribers: (() => void)[] = []
  const reState = { ...state, streams: 1 } as unknown as ConnectionState
  const reConn = {
    getState: () => reState,
    subscribe: (l: () => void) => {
      subscribers.push(l)
      return () => {}
    },
    onEvent: () => () => {},
  } as unknown as Connection
  let current: TicketDetail = {
    ...base,
    status: "claimed",
    stats: { ...base.stats, launch_step: { label: "sending prompt", position: 4, total: 4 } },
  }
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail: current })
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail }), {
    initialEntries: ["/t/gx:demo/12"],
  })
  renderThemed(
    <ConnectionContext.Provider value={reConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  await screen.findByText("Launching: sending prompt (4/4)")

  current = { ...base, status: "claimed" }
  reState.streams = 2
  subscribers.forEach((l) => {
    l()
  })
  await vi.waitFor(() => expect(screen.queryByText(/Launching/)).toBeNull())
})

test("the ticket page shows the ticket's verdict under the header,and nothing without a pending row", async () => {
  const mine: PendingRow = {
    address: "gx:demo/12",
    agent: "",
    verdict: "disk low",
    reason: "/tmp has 5G free",
  }
  await renderTicket(base, undefined, [
    { ...mine, address: "gx:demo/13", verdict: "concurrency cap" },
  ])
  expect(document.querySelector("[data-verdict]")).toBeNull()
  cleanup()

  await renderTicket(base, undefined, [mine])
  expect(screen.getByText("disk low: /tmp has 5G free")).toBeTruthy()
})

const headerActions = () => screen.getByTestId("page-header-actions")

async function openMenu() {
  fireEvent.click(within(headerActions()).getByRole("button", { name: "More actions" }))
  return (await screen.findAllByRole("menuitem")).map((i) => i.textContent?.trim())
}

const oneOff = (status: string): TicketDetail => ({
  ...base,
  address: "proj:one-offs/12",
  epic: "proj:one-offs",
  status,
  actions: [],
})

test("the ticket page has one More actions button, last in the header actions, once loaded", async () => {
  await renderTicket(base)
  const buttons = within(headerActions()).getAllByRole("button", { name: "More actions" })
  expect(buttons).toHaveLength(1)
  expect(headerActions().lastElementChild?.contains(buttons[0]!)).toBe(true)
})

test("a ticket without Start over that is no One-off lists Copy ID only, and Copy ID toasts the address", async () => {
  const writeText = vi.fn(async () => {})
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true })
  await renderTicket({ ...base, status: "claimed", actions: [] })
  expect(await openMenu()).toEqual(["Copy ID"])
  fireEvent.click(screen.getByRole("menuitem", { name: /Copy ID/ }))
  expect(await screen.findByText(`Copied ${base.address}`)).toBeTruthy()
})

test("the menu lists Copy ID then Start over, never a recovery verb", async () => {
  await renderTicket({ ...base, actions: ["retry", "start-over", "resume", "merge"] })
  expect(await openMenu()).toEqual(["Copy ID", "Start over"])
  expect(screen.getAllByRole("separator")).toHaveLength(1)
})

const CANCELLABLE = ["open", "claimed", "needs-answer", "needs-repair"] // oldword:ok

test.each(CANCELLABLE)("a %s One-off lists Cancel run after Start over", async (status) => {
  await renderTicket({ ...oneOff(status), actions: ["start-over"] })
  expect(await openMenu()).toEqual(["Copy ID", "Start over", "Cancel run"])
})

test.each([
  ["a done One-off", oneOff("done")],
  ["a cancelled One-off", oneOff("cancelled")],
  ["an epic ticket", { ...base, status: "claimed", actions: [] } as TicketDetail],
])("%s lists no Cancel run", async (_name, detail) => {
  await renderTicket(detail)
  expect(await openMenu()).toEqual(["Copy ID"])
})

test("Start over and Cancel run from the menu open the shared popover and post nothing yet", async () => {
  const fetchMock = vi.fn(async (_url: string) => new Response(JSON.stringify({})))
  vi.stubGlobal("fetch", fetchMock)
  await renderTicket({ ...oneOff("claimed"), actions: ["start-over"] })
  await openMenu()
  fireEvent.click(screen.getByRole("menuitem", { name: /Start over/ }))
  expect(await screen.findByRole("alertdialog", { name: "Start over" })).toBeTruthy()
  fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" })
  await vi.waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
  await openMenu()
  fireEvent.click(screen.getByRole("menuitem", { name: /Cancel run/ }))
  expect(await screen.findByRole("alertdialog", { name: /Cancel run/ })).toBeTruthy()
  const writes = fetchMock.mock.calls.filter(([url]) => String(url).startsWith("/v1/tickets/"))
  expect(writes).toEqual([])
  vi.unstubAllGlobals()
})

test("while a recovery write is pending the menu's Start over and Cancel run are disabled", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  )
  await renderTicket({ ...oneOff("claimed"), actions: ["start-over"] })
  fireEvent.click(
    within(screen.getByRole("form", { name: "Recovery actions" })).getByRole("button", {
      name: "Cancel run",
    }),
  )
  fireEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", { name: /^Cancel run/ }),
  )
  await vi.waitFor(() =>
    expect(
      within(screen.getByRole("form", { name: "Recovery actions" })).getByRole("button", {
        name: "Cancel run",
      }),
    ).toHaveProperty("disabled", true),
  )
  await openMenu()
  for (const name of [/Start over/, /Cancel run/]) {
    expect(screen.getByRole("menuitem", { name }).hasAttribute("data-disabled")).toBe(true)
  }
  vi.unstubAllGlobals()
})
