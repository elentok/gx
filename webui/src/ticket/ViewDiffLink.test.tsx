import { cleanup, screen } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test } from "vitest"
import canned from "../canned-detail.json"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { TicketDetail } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { FetchDetail } from "./detail"

afterEach(cleanup)

const done: TicketDetail = {
  ...(canned as TicketDetail),
  title: "Finished",
  address: "gx:demo/12",
  epic: "gx:demo",
  map_epic: false,
  status: "done",
  related: [],
  runs: [],
  rounds: [],
  chat: [],
  state: "",
  actions: [],
  permission: undefined,
  answered: false,
  shows_view_diff: true,
  diff_base: "main",
  diff_tip: "ralph-loop/demo",
}

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

async function renderTicket(detail: TicketDetail) {
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail })
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail }), {
    initialEntries: ["/t/gx:demo/12"],
  })
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  await screen.findByRole("link", { name: "Epic: demo" })
}

test("a done ticket links View diff narrowed to its address", async () => {
  await renderTicket(done)
  expect(screen.getByRole("link", { name: "View diff" }).getAttribute("href")).toBe(
    "#/diff?project=gx&base=main&tip=ralph-loop%2Fdemo&epic=gx%3Ademo&ticket=gx%3Ademo%2F12",
  )
})

test("a done commitless ticket has no View diff", async () => {
  await renderTicket({ ...done, shows_view_diff: false, diff_base: undefined, diff_tip: undefined })
  expect(screen.queryByRole("link", { name: "View diff" })).toBeNull()
})
