import { act, cleanup, screen, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test } from "vitest"
import { ConnectionContext, type Connection } from "../connection"
import type { Tip } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { FetchTips } from "./tips"

afterEach(cleanup)

const spotlight: Tip = {
  id: "spotlight",
  title: "Keep worktrees out of Spotlight",
  platform: "darwin",
  body: "Spotlight indexes every worktree.\n1. Open System Settings, then Spotlight.\n2. Open Search Privacy.\n3. Add each folder below.",
  folders: ["/Users/me/dev/gx"],
}

function renderTips(tips: FetchTips) {
  const router = createMemoryRouter(appRoutes({ tips }), { initialEntries: ["/tips"] })
  const state = {
    snapshot: undefined,
    connected: true,
    loadedOnce: true,
    liveSnapshot: true,
    tick: 0,
    streams: 0,
  }
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
}

test("spinner before data, then the Spotlight tip with steps and folders and no done badge", async () => {
  let resolve!: (t: Tip[]) => void
  renderTips(() => new Promise((r) => (resolve = r)))
  expect(await screen.findByRole("status", { name: "Loading tips" })).toBeTruthy()
  await act(async () => resolve([spotlight]))
  const region = await screen.findByRole("region", { name: "Keep worktrees out of Spotlight" })
  expect(within(region).getByText(/Open System Settings, then Spotlight/)).toBeTruthy()
  expect(within(region).getByText(/Add each folder below/)).toBeTruthy()
  expect(within(region).getByRole("listitem").textContent).toBe("/Users/me/dev/gx")
  expect(screen.queryByText(/done/i)).toBeNull()
})

test("an empty list says there are no tips for this machine", async () => {
  renderTips(async () => [])
  expect(await screen.findByText("No tips for this machine")).toBeTruthy()
})
