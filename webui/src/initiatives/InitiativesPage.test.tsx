import { fireEvent, screen, within } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { App } from "../App"
import type { InitiativeList } from "../generated/server"
import type { Snapshot } from "../snapshot"
import { renderThemed } from "../ui/render"
import { ACTIVE_KEY } from "../workspaces/active"

const snapshot = {
  seq: 1,
  budget: {
    day: "",
    total: 0,
    soft_limit: 0,
    hard_limit: 0,
    budget_paused: false,
    hard_latched: false,
  },
  health: {
    cpu: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    memory: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    disk: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
  },
  tickets: [],
  pending: [],
  sessions: [],
  epics: [],
  recently_finished: [],
} as unknown as Snapshot

const row = {
  done: 1,
  total: 3,
  current: "",
  current_number: 0,
  current_state: "",
  stopped_at: 0,
}

const list: InitiativeList = {
  projects: [
    {
      project: "gx",
      workspaces: ["Default"],
      initiatives: [
        {
          ...row,
          address: "gx:payments-rewrite",
          slug: "payments-rewrite",
          state: "active",
          current: "refunds",
          current_number: 2,
          current_state: "designing",
        },
        { ...row, address: "gx:mobile-app", slug: "mobile-app", state: "stopped", stopped_at: 1 },
        { ...row, address: "gx:old-rewrite", slug: "old-rewrite", state: "finished", done: 3 },
      ],
    },
    { project: "other", workspaces: ["Else"], initiatives: [] },
  ],
}

function renderInitiatives(answer: () => Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => answer()),
  )
  window.location.hash = "#/initiatives"
  renderThemed(
    <App
      deps={{
        cache: { load: () => snapshot, save: () => {} },
        fetchSnapshot: async () => snapshot,
        openStream: () => new Promise(() => {}),
      }}
    />,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.removeItem(ACTIVE_KEY)
  window.location.hash = ""
})

test("lists Initiatives per project with state, progress and the current Milestone, and no button", async () => {
  renderInitiatives(() => new Response(JSON.stringify(list)))

  const region = await screen.findByRole("region", { name: "gx" })
  expect(screen.queryByRole("region", { name: "other" })).toBeNull()
  const rows = within(region).getAllByRole("row").slice(1)
  expect(
    rows.map((r) =>
      within(r)
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ),
  ).toEqual([
    ["payments-rewrite", "Active", "1 of 3 done", "Milestone 2: refunds (designing)"],
    ["mobile-app", "Stopped", "1 of 3 done", "Stopped at Milestone 1 gate"],
    ["old-rewrite", "Finished", "3 of 3 done", "All Milestones gated"],
  ])
  expect(within(region).queryAllByRole("button")).toEqual([])
  expect(within(region).getByRole("link", { name: "payments-rewrite" }).getAttribute("href")).toBe(
    "#/initiatives/gx:payments-rewrite",
  )
})

test("with no Initiative anywhere the page says how to start one", async () => {
  renderInitiatives(() => new Response(JSON.stringify({ projects: [] })))

  expect(await screen.findByText(/gx initiatives new/)).toBeTruthy()
})

test("a failed load shows the failure line and Retry loads again", async () => {
  let ok = false
  renderInitiatives(() =>
    ok ? new Response(JSON.stringify(list)) : new Response("", { status: 500 }),
  )

  expect(await screen.findByText("Could not load the initiatives")).toBeTruthy()
  ok = true
  fireEvent.click(screen.getByRole("button", { name: "Retry" }))
  expect(await screen.findByRole("region", { name: "gx" })).toBeTruthy()
})

test("the sidebar workspace filter does not hide projects", async () => {
  localStorage.setItem(ACTIVE_KEY, "Else")
  renderInitiatives(() => new Response(JSON.stringify(list)))

  expect(await screen.findByRole("region", { name: "gx" })).toBeTruthy()
})
