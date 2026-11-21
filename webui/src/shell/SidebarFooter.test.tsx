import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { renderThemed } from "../ui/render"
import { SidebarFooter } from "./SidebarFooter"

afterEach(cleanup)

const startNewDesign = vi.hoisted(() => vi.fn())
const startNewChatDraft = vi.hoisted(() => vi.fn())
vi.mock("../design/newDesign", () => ({ startNewDesign, startNewChatDraft }))

const meter = (state: string) => ({ level: 1, state, read_at: "" })

function setup(
  snapshot: Record<string, unknown> = {},
  connected = true,
  onNewWorkspace?: () => void,
) {
  const state = { snapshot, connected, loadedOnce: true, tick: 0 } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const router = createMemoryRouter([
    { path: "/", element: <SidebarFooter onNewWorkspace={onNewWorkspace} /> },
  ])
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
}

const status = () => document.querySelector<HTMLElement>('[data-part="status"]')

test("the status button hides when nothing is wrong", () => {
  setup({
    mode: "running",
    health: { cpu: meter("normal"), memory: meter("normal"), disk: meter("normal") },
  })
  expect(status()).toBeNull()
})

test("the status button lists every problem and opens a menu saying why", async () => {
  setup(
    {
      mode: "paused",
      health: { cpu: meter("normal"), memory: meter("critical"), disk: meter("normal") },
    },
    false,
  )
  const button = status()!
  expect(button.getAttribute("title")).toContain("System health is high or critical")
  expect(button.getAttribute("title")).toContain("Server queue is paused")
  expect(button.getAttribute("title")).toContain("Disconnected from the server")
  fireEvent.click(button)
  expect(await screen.findByText("memory is critical.", { exact: false })).toBeTruthy()
  expect(screen.getByText(/No new tickets start/)).toBeTruthy()
})

test("Dev mode alone adds no indicator to the footer", () => {
  setup({ mode: "running", dev: { state: "running", has_checkout: true } })
  expect(status()).toBeNull()
  expect(document.querySelector('[data-part="dev-pill"]')).toBeNull()
})

test("the + New menu's New design item calls the action once", async () => {
  startNewDesign.mockClear()
  setup()
  fireEvent.click(screen.getByRole("button", { name: "+ New" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "New design" }))
  await waitFor(() => expect(startNewDesign).toHaveBeenCalledTimes(1))
})

test("the + New menu lists the three items and New workspace calls its prop", async () => {
  const onNewWorkspace = vi.fn()
  setup({}, true, onNewWorkspace)
  fireEvent.click(screen.getByRole("button", { name: "+ New" }))
  await screen.findByRole("menuitem", { name: "New design" })
  expect(screen.getAllByRole("menuitem").map((el) => el.textContent)).toEqual([
    "New design",
    "New chat",
    "New chat draft",
    "New workspace…",
  ])
  fireEvent.click(screen.getByRole("menuitem", { name: "New chat draft" }))
  await waitFor(() => expect(startNewChatDraft).toHaveBeenCalledTimes(1))
  fireEvent.click(screen.getByRole("button", { name: "+ New" }))
  fireEvent.click(screen.getByRole("menuitem", { name: "New workspace…" }))
  await waitFor(() => expect(onNewWorkspace).toHaveBeenCalled())
})

test("the Epics badge shows the server total and is absent at zero", () => {
  setup({ needs_you_total: 3 })
  expect(screen.getByRole("button", { name: "Epics · 3 need you" })).toBeTruthy()
  cleanup()
  setup({ needs_you_total: 0 })
  expect(screen.getByRole("button", { name: "Epics" })).toBeTruthy()
})

test("the Epics icon opens a menu with Epics and Archive, and Archive navigates to /archive", async () => {
  const router = setupWithRouter()
  fireEvent.click(screen.getByRole("button", { name: "Epics" }))
  expect(await screen.findByRole("menuitem", { name: "Epics" })).toBeTruthy()
  fireEvent.click(screen.getByRole("menuitem", { name: "Archive" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/archive"))
})

test("the Epics menu lists Backlog and Initiatives before the one separator and Scheduled prompts after it, and it navigates to /schedules", async () => {
  const router = setupWithRouter()
  fireEvent.click(screen.getByRole("button", { name: "Epics" }))
  const menu = await screen.findByRole("menu")
  await waitFor(() => expect(menu.querySelectorAll('[role="separator"]').length).toBe(1))
  const names = Array.from(menu.querySelectorAll('[role="menuitem"],[role="separator"]')).map(
    (el) => (el.getAttribute("role") === "separator" ? "---" : el.textContent?.replace(/\d+$/, "")),
  )
  expect(names).toEqual(["Epics", "Archive", "Backlog", "Initiatives", "---", "Scheduled prompts"])
  fireEvent.click(screen.getByRole("menuitem", { name: "Scheduled prompts" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/schedules"))
})

test("the Initiatives menu item navigates to /initiatives", async () => {
  const router = setupWithRouter()
  fireEvent.click(screen.getByRole("button", { name: "Epics" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Initiatives" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/initiatives"))
})

test("the Epics menu entry shows the epic count", async () => {
  const epics = [{ address: "gx:a" }, { address: "gx:b" }, { address: "gx:c" }]
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ epics, follow_ups: [] }))),
  )
  setupWithRouter()
  fireEvent.click(screen.getByRole("button", { name: "Epics" }))
  const item = await screen.findByRole("menuitem", { name: /^Epics/ })
  await waitFor(() => expect(item.textContent).toBe("Epics3"))
  vi.unstubAllGlobals()
})

function stubUsage(ok: boolean) {
  const summary = {
    epics: [],
    follow_ups: [],
    mode: "limits",
    limits: {
      five_hour: {
        used_percentage: 10,
        resets_at: Date.now() / 1000 + 3600,
        received_at: Date.now() / 1000,
      },
    },
  }
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      ok ? new Response(JSON.stringify(summary)) : new Response("", { status: 500 }),
    ),
  )
}

test("the limits tray renders without a health snapshot, and nothing renders without usage", async () => {
  stubUsage(true)
  setup({ mode: "running" })
  expect(await screen.findByRole("button", { name: "Usage limits" })).toBeTruthy()
  cleanup()
  stubUsage(false)
  setup({ mode: "running" })
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled())
  expect(screen.queryByRole("button", { name: "Usage limits" })).toBeNull()
  vi.unstubAllGlobals()
})

const followUps = (over: Record<string, unknown>) => ({
  project: "gx",
  open: 3,
  bugs: 2,
  ideas: 1,
  other: 0,
  triage: "none",
  left: 0,
  ...over,
})

function stubList(follow_ups: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ epics: [], follow_ups }))),
  )
}

function setupWithRouter(snapshot: Record<string, unknown> = {}) {
  const state = {
    snapshot,
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const router = createMemoryRouter([
    { path: "/", element: <SidebarFooter /> },
    { path: "/epics", element: <div>epics page</div> },
    { path: "/archive", element: <div>archive page</div> },
    { path: "/triage/:project", element: <div>triage page</div> },
  ])
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

test("the follow-ups button sums the projects and its popover has a line per project", async () => {
  stubList([
    followUps({ open: 12, bugs: 8, ideas: 4 }),
    followUps({ project: "webapp", open: 4, bugs: 3, ideas: 1 }),
  ])
  setupWithRouter()
  fireEvent.click(await screen.findByRole("button", { name: "Follow-ups · 16 open" }))
  expect(await screen.findByRole("menuitem", { name: /gx.*8 bugs · 4 ideas/ })).toBeTruthy()
  expect(screen.getByRole("menuitem", { name: /webapp.*3 bugs · 1 idea/ })).toBeTruthy()
  vi.unstubAllGlobals()
})

test("a line opens the Epics page with the card focus, and a live Triage line opens the Triage page", async () => {
  stubList([followUps({}), followUps({ project: "webapp", triage: "waiting", left: 12 })])
  const router = setupWithRouter()
  fireEvent.click(await screen.findByRole("button", { name: /^Follow-ups/ }))
  fireEvent.click(await screen.findByRole("menuitem", { name: /webapp.*Triage · 12 left/ }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/triage/webapp"))
  await router.navigate("/")
  fireEvent.click(await screen.findByRole("button", { name: /^Follow-ups/ }))
  fireEvent.click(await screen.findByRole("menuitem", { name: /gx.*2 bugs/ }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/epics"))
  expect(router.state.location.state).toEqual({ focus: "follow-ups:gx" })
  vi.unstubAllGlobals()
})

test("the follow-ups badge has no number at zero, and is amber for a waiting entry only", async () => {
  stubList([followUps({ open: 0, bugs: 0, ideas: 0 })])
  setupWithRouter()
  const zero = await screen.findByRole("button", { name: "Follow-ups" })
  expect(zero.textContent).toBe("")
  cleanup()
  stubList([followUps({ triage: "waiting", left: 2 })])
  setupWithRouter()
  const waiting = await screen.findByRole("button", { name: "Follow-ups · 3 open" })
  expect(waiting.querySelector("span")?.className).not.toBe(undefined)
  const waitingClass = waiting.querySelector("span")!.className
  cleanup()
  stubList([followUps({ triage: "running", left: 2 })])
  setupWithRouter()
  const running = await screen.findByRole("button", { name: "Follow-ups · 3 open" })
  expect(running.querySelector("span")!.className).not.toBe(waitingClass)
  vi.unstubAllGlobals()
})

test("the follow-ups badge takes its colours from theme tokens, so a light theme restyles it", async () => {
  stubList([followUps({ triage: "waiting", left: 2 })])
  setupWithRouter()
  const button = await screen.findByRole("button", { name: "Follow-ups · 3 open" })
  const badge = button.querySelector("span")!
  const css = [...document.querySelectorAll("style")].map((s) => s.textContent).join("\n")
  const rule =
    css.match(new RegExp(`\\.${badge.className.split(" ").join("\\.")}\\s*\\{[^}]*\\}`))?.[0] ?? ""
  expect(rule).toContain("var(--chakra-colors-tone-waiting-fg)")
  expect(rule).toContain("var(--chakra-colors-surface)")
  expect(rule).not.toMatch(/#[0-9a-f]{3,8}\b/i)
  vi.unstubAllGlobals()
})

test("a live Triage line draws a partial ring: amber, with a hairline top arc", async () => {
  stubList([followUps({ project: "webapp", triage: "waiting", left: 12 })])
  setupWithRouter()
  fireEvent.click(await screen.findByRole("button", { name: /^Follow-ups/ }))
  const ring = await screen.findByTestId("triage-ring")
  const css = [...document.querySelectorAll("style")].map((s) => s.textContent).join("\n")
  const rule =
    css.match(new RegExp(`\\.${ring.className.split(" ").join("\\.")}\\s*\\{[^}]*\\}`))?.[0] ?? ""
  expect(rule).toMatch(/border-color:\s*var\(--chakra-colors-tone-waiting-fg\)/)
  expect(rule).toMatch(/border-top-color:\s*var\(--chakra-colors-hairline\)/)
  vi.unstubAllGlobals()
})

test("the health button and the gear carry a title tooltip", () => {
  setup({
    mode: "running",
    health: { cpu: meter("normal"), memory: meter("normal"), disk: meter("normal") },
  })
  expect(screen.getByRole("button", { name: /^System health:/ }).getAttribute("title")).toMatch(
    /^System health:/,
  )
  expect(screen.getByRole("button", { name: "Menu" }).getAttribute("title")).toBe("Menu")
})

test("the Epics and Follow-ups badges show their count text", async () => {
  stubList([followUps({ triage: "running", left: 2 })])
  setupWithRouter({ needs_you_total: 2 })
  const epics = (await screen.findByRole("button", { name: "Epics · 2 need you" })).querySelector(
    "[data-part=badge]",
  )!
  const follow = (await screen.findByRole("button", { name: "Follow-ups · 3 open" })).querySelector(
    "[data-part=badge]",
  )!
  expect(epics.textContent).toBe("2")
  expect(follow.textContent).toBe("3")
  vi.unstubAllGlobals()
})

test("every footer box is present with its label", () => {
  setup(
    {
      mode: "paused",
      needs_you_total: 1,
      health: { cpu: meter("normal"), memory: meter("critical"), disk: meter("normal") },
    },
    false,
  )
  expect(screen.getByRole("button", { name: /^Epics/ })).toBeTruthy()
  expect(screen.getByRole("button", { name: /^Follow-ups/ })).toBeTruthy()
  expect(screen.getByRole("button", { name: "Menu" })).toBeTruthy()
  expect(status()).not.toBeNull()
})
