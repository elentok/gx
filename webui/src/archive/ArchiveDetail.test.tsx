import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import type { EpicList } from "../epics/list"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { EpicDetail } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"

const bulkUnarchive = vi.hoisted(() => vi.fn())
// bulkUnarchive is called inside unarchiveWithToasts, so the stub sits at the POST it makes.
vi.mock("../design/open", async (orig) => ({
  ...(await orig<object>()),
  postTargetsTo: (_path: string, body: { targets: string[] }) => bulkUnarchive(body.targets),
}))

afterEach(() => {
  cleanup()
  bulkUnarchive.mockReset()
})

const detail = (over: Partial<EpicDetail> = {}): EpicDetail => ({
  address: "gx:blog-impl",
  kind: "loop",
  summary: "",
  done: 1,
  total: 1,
  agent_time: 0,
  cost_total: 0,
  stats: {},
  rows: [
    { address: "gx:blog-impl/01", title: "title 01", rendered_status: "done" },
  ] as EpicDetail["rows"],
  queueable: true,
  queued: false,
  state: "finished",
  archived_at: "2026-03-04T10:00:00Z",
  ...over,
})

function render(
  path: string,
  d: EpicDetail | "not-found",
  activeList: EpicList = { epics: [], follow_ups: [] },
) {
  const state = {
    snapshot: { sessions: [], pending: [] },
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
      epicList: async () => activeList,
      // The active fetcher must stay unused on this route.
      epic: async () => {
        throw new Error("active detail fetched")
      },
      archivedEpic: async () =>
        d === "not-found" ? { kind: "not-found" } : { kind: "ok", detail: d },
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

test("shows the Archived stamp and Unarchive as the only action", async () => {
  render("/archive/gx:blog-impl", detail())
  expect(await screen.findByTestId("archived-stamp")).toBeTruthy()
  const names = screen.getAllByRole("button").map((b) => b.textContent)
  expect(names).toContain("Unarchive")
  for (const gone of ["Queue", "Hold", "Merge", "Fix", "Open design"]) {
    expect(names.some((n) => n?.startsWith(gone))).toBe(false)
  }
})

test("ticket rows are plain text, not links", async () => {
  render("/archive/gx:blog-impl", detail())
  expect(await screen.findByText("title 01")).toBeTruthy()
  expect(screen.queryByRole("link", { name: /title 01/ })).toBeNull()
})

test("an archived Design's -impl button goes to the archive detail", async () => {
  const router = render(
    "/archive/gx:blog",
    detail({ address: "gx:blog", kind: "map", impl_address: "gx:blog-impl", rows: [], total: 0 }),
  )
  fireEvent.click(await screen.findByRole("button", { name: "Open implementation epic" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/archive/gx:blog-impl"))
})

test("Unarchive posts and navigates to the renamed active epic", async () => {
  bulkUnarchive.mockResolvedValue([
    { target: "gx:blog-impl", status: "done", new_address: "gx:blog-impl-2" },
  ])
  const router = render("/archive/gx:blog-impl", detail())
  fireEvent.click(await screen.findByRole("button", { name: "Unarchive" }))
  await waitFor(() => expect(router.state.location.pathname).toBe("/e/gx:blog-impl-2"))
  expect(bulkUnarchive).toHaveBeenCalledWith(["gx:blog-impl"])
})

test("the ring comes from the archived detail, not an active epic with the same address", async () => {
  const active = {
    address: "gx:blog-impl",
    addresses: ["gx:blog-impl"],
    title: "blog",
    state: "running",
    section: "running",
    done: 0,
    total: 1,
  } as unknown as EpicList["epics"][number]
  render("/archive/gx:blog-impl", detail(), { epics: [active], follow_ups: [] })
  await screen.findByTestId("archived-stamp")
  const ring = within(screen.getByTestId("page-header-actions"))
    .getAllByText(/\d\/\d/)[0]
    .closest("[data-tone]")
  expect(ring?.getAttribute("data-tone")).toBe("done")
})

test("an unknown address shows the not-found state", async () => {
  render("/archive/gx:nope", "not-found")
  expect(await screen.findByText("Epic not found")).toBeTruthy()
})
