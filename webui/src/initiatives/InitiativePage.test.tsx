import { fireEvent, screen, within } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { App } from "../App"
import type { InitiativeMilestone, InitiativePage } from "../generated/server"
import type { Snapshot } from "../snapshot"
import { renderThemed } from "../ui/render"

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

const ms = (over: Partial<InitiativeMilestone>): InitiativeMilestone => ({
  number: 1,
  slug: "m",
  name: "M",
  goal: "goal",
  state: "sketch",
  broken: false,
  design: "",
  design_address: "",
  impl_address: "",
  impl_exists: false,
  design_archived: false,
  impl_archived: false,
  ...over,
})

const page: InitiativePage = {
  address: "gx:payments-rewrite",
  slug: "payments-rewrite",
  state: "active",
  stopped_at: 0,
  destination: "Charge customers.\n\nRefund them too.",
  milestones: [
    ms({
      number: 1,
      name: "Payment core",
      goal: "Charge one card",
      state: "implementing",
      design: "pay-core",
      design_address: "gx:pay-core",
      impl_address: "gx:pay-core-impl",
      impl_exists: true,
    }),
    ms({ number: 2, name: "Refunds", goal: "Refund a charge" }),
    ms({
      number: 3,
      name: "Invoices",
      goal: "Monthly",
      state: "designing",
      design: "inv",
      design_address: "gx:inv",
      impl_address: "gx:inv-impl",
    }),
    ms({
      number: 4,
      name: "Old",
      goal: "Archived",
      state: "merged",
      design: "old",
      design_address: "gx:old",
      impl_address: "gx:old-impl",
      impl_exists: true,
      design_archived: true,
      impl_archived: true,
    }),
    ms({ number: 5, name: "Lost", goal: "Missing", state: "broken", broken: true, design: "gone" }),
  ],
  assumptions: [
    { n: 1, text: "Keys work", tested_by: "pay-core", state: "confirmed", evidence: "PR 812" },
    { n: 2, text: "Ordered", tested_by: "refunds", state: "broken", evidence: "seen twice" },
  ],
  deferred: [
    { item: "Retry queue", to: "refunds", why: "sandbox", from: "pay-core", state: "open" },
    { item: "Receipts", to: "unplaced", why: "no fit", from: "pay-core", state: "taken" },
  ],
  gate_notes: [
    {
      slug: "pay-core",
      outcome: "continue",
      assumptions: [{ n: 1, state: "confirmed", evidence: "PR 812" }],
      taught: ["Keys work"],
      placed: ["Retry queue to refunds"],
    },
  ],
  unparsed: [],
}

function renderPage(answer: (url: string) => Response, address = "gx:payments-rewrite") {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => answer(String(url))),
  )
  window.location.hash = `#/initiatives/${address}`
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

const ok = (p: InitiativePage) => () => new Response(JSON.stringify(p))

afterEach(() => {
  vi.unstubAllGlobals()
  window.location.hash = ""
})

test("the sections come in order in one column, under a header with the slug and state pill", async () => {
  renderPage(ok(page))

  expect(await screen.findByText("Active")).toBeTruthy()
  expect(screen.getByRole("heading", { name: "payments-rewrite" })).toBeTruthy()
  const names = ["Destination", "Milestones", "Assumptions", "Deferred", "Gate notes"]
  const sections = screen
    .getAllByRole("region")
    .map((r) => r.getAttribute("aria-label") ?? "")
    .filter((n) => names.includes(n))
  expect(sections).toEqual(names)
  expect(screen.getByText("Refund them too.")).toBeTruthy()
})

test("a Milestone shows its state, goal and links; a sketch has none and a missing Design is plain text", async () => {
  renderPage(ok(page))

  const region = await screen.findByRole("region", { name: "Milestones" })
  const link = (name: string) => within(region).getByRole("link", { name }).getAttribute("href")
  expect(link("Design: pay-core")).toBe("#/d/gx:pay-core")
  expect(link("Impl epic: pay-core-impl")).toBe("#/e/gx:pay-core-impl")
  expect(within(region).getByText("1. Payment core")).toBeTruthy()
  expect(within(region).getByText("implementing")).toBeTruthy()
  expect(within(region).getByText("Charge one card")).toBeTruthy()
  expect(within(region).getByText("Sketch only: no Design yet")).toBeTruthy()
  // Designing: only the Design link, the impl epic is not there yet.
  expect(link("Design: inv")).toBe("#/d/gx:inv")
  expect(within(region).queryByRole("link", { name: "Impl epic: inv-impl" })).toBeNull()
  // Archived epics keep their links, pointing at the Archive page.
  expect(link("Design: old")).toBe("#/archive/gx:old")
  expect(link("Impl epic: old-impl")).toBe("#/archive/gx:old-impl")
  // Missing Design: marker and slug, no link.
  expect(within(region).getByText("broken link")).toBeTruthy()
  expect(within(region).queryByRole("link", { name: /gone/ })).toBeNull()
  expect(within(region).getByText(/Design: gone/)).toBeTruthy()
})

test("the Assumptions and Deferred tables show every column", async () => {
  renderPage(ok(page))

  const cells = (name: string) =>
    within(screen.getByRole("region", { name }))
      .getAllByRole("row")
      .map((r) => [...r.querySelectorAll("th,td")].map((c) => c.textContent))
  await screen.findByRole("region", { name: "Assumptions" })
  expect(cells("Assumptions")).toEqual([
    ["#", "Assumption", "Tested by", "State", "Evidence"],
    ["A1", "Keys work", "pay-core", "confirmed", "PR 812"],
    ["A2", "Ordered", "refunds", "broken", "seen twice"],
  ])
  expect(cells("Deferred")).toEqual([
    ["Item", "To", "Why", "From", "State"],
    ["Retry queue", "refunds", "sandbox", "pay-core", "open"],
    ["Receipts", "unplaced", "no fit", "pay-core", "taken"],
  ])
})

test("a gate note shows the outcome, Assumption lines, taught and placements", async () => {
  renderPage(ok(page))

  const region = await screen.findByRole("region", { name: "Gate notes" })
  expect(region.textContent).toContain("pay-core · outcome continue")
  expect(region.textContent).toContain("A1 · confirmed · PR 812")
  expect(region.textContent).toContain("Taught: Keys work")
  expect(region.textContent).toContain("Placements: Retry queue to refunds")
})

test("empty lists show empty-state lines, and Gate notes names /gx-initiative with the address", async () => {
  renderPage(ok({ ...page, destination: "", assumptions: [], deferred: [], gate_notes: [] }))

  expect(await screen.findByText("Nothing deferred.")).toBeTruthy()
  expect(screen.getByText("No Assumptions yet.")).toBeTruthy()
  expect(screen.getByText("No Destination written yet.")).toBeTruthy()
  expect(screen.getByText("/gx-initiative gx:payments-rewrite")).toBeTruthy()
})

test("an unparsed line shows verbatim in each section and drops nothing", async () => {
  renderPage(
    ok({
      ...page,
      unparsed: [
        { section: "Milestones", raw: "bad milestone" },
        { section: "Assumptions", raw: "bad assumption" },
        { section: "Deferred", raw: "bad deferred" },
        { section: "Gate notes", raw: "bad gate line" },
      ],
    }),
  )

  await screen.findByRole("region", { name: "Milestones" })
  for (const [name, raw] of [
    ["Milestones", "bad milestone"],
    ["Assumptions", "bad assumption"],
    ["Deferred", "bad deferred"],
    ["Gate notes", "bad gate line"],
  ]) {
    expect(within(screen.getByRole("region", { name })).getByText(raw)).toBeTruthy()
  }
  expect(screen.getByText("1. Payment core")).toBeTruthy()
  expect(
    within(screen.getByRole("region", { name: "Assumptions" })).getByText("Keys work"),
  ).toBeTruthy()
})

test("an unparsed line alone keeps its section out of the empty state", async () => {
  renderPage(
    ok({
      ...page,
      assumptions: [],
      deferred: [],
      gate_notes: [],
      unparsed: [{ section: "Gate notes", raw: "bad gate line" }],
    }),
  )

  expect(await screen.findByText("bad gate line")).toBeTruthy()
  expect(screen.queryByText(/No gate yet/)).toBeNull()
})

test("the page has no button", async () => {
  renderPage(ok(page))

  const body = await screen.findByRole("region", { name: "Milestones" })
  expect(within(body).queryAllByRole("button")).toEqual([])
  for (const name of ["Destination", "Assumptions", "Deferred", "Gate notes"]) {
    expect(within(screen.getByRole("region", { name })).queryAllByRole("button")).toEqual([])
  }
})

test("an unknown address shows the not-found state", async () => {
  renderPage(() => new Response("", { status: 404 }), "gx:nope")

  expect(await screen.findByText("Initiative not found")).toBeTruthy()
})

test("a failed load shows the failure line and Retry loads again", async () => {
  let up = false
  renderPage(() => (up ? new Response(JSON.stringify(page)) : new Response("", { status: 500 })))

  expect(await screen.findByText("Could not load the initiative")).toBeTruthy()
  up = true
  fireEvent.click(screen.getByRole("button", { name: "Retry" }))
  expect(await screen.findByRole("region", { name: "Milestones" })).toBeTruthy()
})
