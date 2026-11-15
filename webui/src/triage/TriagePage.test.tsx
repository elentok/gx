import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { MapStep, SessionInfo, TriageView } from "../generated/server"
import { appRoutes, type RouteFetchers } from "../routes"
import { roundCards } from "../design/roundFixture"
import { renderThemed } from "../ui/render"

afterEach(cleanup)

const STEPS: [string, string][] = [
  ["verify", "Verify"],
  ["route", "Route"],
  ["questions", "Questions"],
  ["write", "Write"],
  ["done", "Done"],
]
const stepsAt = (current: number): MapStep[] =>
  STEPS.map(([step, label], i) => ({
    step,
    label,
    state: i < current ? "done" : i === current ? "current" : "todo",
  }))

const row = (n: number, verdict?: string) => ({
  address: `gx:follow-ups/${n}`,
  title: `Row ${n}`,
  verdict,
})

const viewOf = (over: Partial<TriageView> = {}): TriageView => ({
  project: "gx",
  state: "live",
  flow: "triage",
  steps: stepsAt(0),
  step_note: "2 of 3 verdicts saved",
  ring_percent: 20,
  rows: [row(1, "relevant"), row(2, "obsolete"), row(3)],
  open_questions: 0,
  action_counts: {},
  changed_rows: 0,
  destinations: [],
  epic_actions: ["ready", "map"],
  can_abandon: false,
  history: [],
  ...over,
})

const session = (
  state: "idle" | "working" | "exited",
  over: Partial<SessionInfo> = {},
): SessionInfo => ({
  id: "s-triage",
  project: "gx",
  dir: "/x",
  harness: "claude",
  flow: "triage",
  flow_key: "gx",
  status: { state },
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  auto_advance: false,
  auto_queue: false,
  ...over,
})

function render(sessions: SessionInfo[], fetchers: RouteFetchers = {}) {
  const state = {
    snapshot: { sessions },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const events = new Set<(address: string | undefined, type?: string) => void>()
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: (address: string | undefined, type?: string) => void) => (
      events.add(l), () => events.delete(l)
    ),
    refresh: () => {},
  } as unknown as Connection
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({ epics: [], follow_ups: [] }),
      triage: async () => ({ kind: "ok", detail: viewOf() }),
      ...fetchers,
    }),
    { initialEntries: ["/triage/gx"] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return {
    emit: (address: string, type = "triage-changed") =>
      act(() => events.forEach((l) => l(address, type))),
  }
}

const onPhone = async (body: () => Promise<void>) => {
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
  }))
  try {
    await body()
  } finally {
    vi.unstubAllGlobals()
  }
}

test("the Verify header shows the title, the step summary, five steps with the first current, and the follow-ups link", async () => {
  render([session("working")])
  expect((await screen.findByRole("heading", { level: 1 })).textContent).toBe("Triage: gx")
  expect((await screen.findByTestId("page-header-subtitle")).textContent).toBe("Verify")
  const steps = document.querySelectorAll("[data-step]")
  expect([...steps].map((s) => s.getAttribute("data-step"))).toEqual([
    "verify",
    "route",
    "questions",
    "write",
    "done",
  ])
  expect(steps[0].getAttribute("data-state")).toBe("current")
  expect(screen.getByRole("link", { name: "Triage of gx:follow-ups ↗" }).getAttribute("href")).toBe(
    "#/e/gx:follow-ups",
  )
  expect(document.title).toBe("gx · Triage · gx")
})

test("no banner shows while a session runs, even with a step note", async () => {
  render([session("working")])
  await screen.findByRole("heading", { level: 1 })
  expect(screen.queryByText(/Triage session running/)).toBeNull()
})

test("at Write Continue follows the session", async () => {
  const rows = [
    { ...row(1), write_state: "done" },
    {
      ...row(2),
      write_state: "failed",
      refusal: "close: ticket is claimed, not draft or open",
      can_retry: true,
    },
    { ...row(3), write_state: "pending" },
  ]
  const detail = viewOf({ steps: stepsAt(3), rows })
  render([session("working")], { triage: async () => ({ kind: "ok", detail }) })
  await screen.findByText("close: ticket is claimed, not draft or open")
  expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(
    true,
  )
  cleanup()
  render([session("exited")], { triage: async () => ({ kind: "ok", detail }) })
  expect(
    ((await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement).disabled,
  ).toBe(false)
})

test("Continue is disabled while a session runs and enabled once it is stopped, and posts the open route", async () => {
  const openTriage = vi.fn(async () => ({}))
  render([session("working")], { openTriage })
  const button = (await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement
  expect(button.disabled).toBe(true)
  cleanup()
  render([session("exited")], { openTriage })
  const stopped = (await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement
  expect(stopped.disabled).toBe(false)
  fireEvent.click(stopped)
  await waitFor(() => expect(openTriage).toHaveBeenCalledWith({ epic: "gx" }))
})

test("an idle session's Continue asks for a fresh session", async () => {
  const openTriage = vi.fn(async () => ({}))
  const idle = session("idle")
  render([idle], { openTriage })
  const button = (await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement
  expect(button.disabled).toBe(false)
  fireEvent.click(button)
  await waitFor(() =>
    expect(openTriage).toHaveBeenCalledWith({
      epic: "gx",
      new: true,
      state: expect.stringMatching(/^s-triage:idle:/),
    }),
  )
})

test.each([1, 2])("at step %i the page is chat only, with Continue and no Approve", async (at) => {
  const detail = viewOf({ steps: stepsAt(at), open_questions: 3 })
  render([session("idle")], { triage: async () => ({ kind: "ok", detail }) })
  expect(
    ((await screen.findByRole("button", { name: "Continue" })) as HTMLButtonElement).disabled,
  ).toBe(false)
  expect(screen.queryByRole("button", { name: /Approve/ })).toBeNull()
  expect((await screen.findByTestId("page-header-subtitle")).textContent).toBe(STEPS[at][1])
})

test("with no session the header button reads Start", async () => {
  render([])
  expect(
    ((await screen.findByRole("button", { name: "Start" })) as HTMLButtonElement).disabled,
  ).toBe(false)
})

test("a triage-changed event for the project refetches the view", async () => {
  const triage = vi.fn(async () => ({ kind: "ok" as const, detail: viewOf() }))
  const { emit } = render([session("working")], { triage })
  await screen.findByRole("heading", { level: 1 })
  const calls = triage.mock.calls.length
  emit("gx")
  await waitFor(() => expect(triage.mock.calls.length).toBeGreaterThan(calls))
})

test("on a phone there is no Progress panel", () =>
  onPhone(async () => {
    render([session("working")])
    await screen.findByRole("heading", { level: 1 })
    expect(screen.queryByRole("region", { name: "Progress" })).toBeNull()
    expect(screen.getByRole("button", { name: "Show progress steps" })).toBeTruthy()
    fireEvent.click(document.querySelector('button[aria-haspopup="dialog"]:not([aria-label])')!)
    const sheet = await screen.findByRole("dialog", { name: "Triage progress steps" })
    expect(
      [...sheet.querySelectorAll("[data-step]")].map((s) => s.getAttribute("data-step")),
    ).toEqual(["verify", "route", "questions", "write", "done"])
  }))

test("a wide page shows the Progress panel", async () => {
  render([session("working")])
  await screen.findByRole("heading", { level: 1 })
  expect(screen.queryByRole("region", { name: "Progress" })).toBeTruthy()
})

test("a project with no record shows the empty state", async () => {
  render([], { triage: async () => ({ kind: "not-found" }) })
  await screen.findByText("No Triage for gx")
  cleanup()
  render([], { triage: async () => ({ kind: "ok", detail: viewOf({ state: "none" }) }) })
  await screen.findByText("No Triage for gx")
})

type Result = TriageView["history"][number]["result"]
const past = (id: string, started: string, state = "done", result: Result = { groups: [] }) => ({
  id,
  state,
  started_at: started,
  rows: 3,
  result,
})

const openMenu = async () =>
  fireEvent.click(await screen.findByRole("button", { name: "More actions" }))

test("the menu offers Abandon only when can_abandon is true", async () => {
  render([session("working")], {
    triage: async () => ({ kind: "ok", detail: viewOf({ can_abandon: true }) }),
  })
  await openMenu()
  await screen.findByRole("menuitem", { name: /Abandon Triage/ })
  cleanup()
  render([session("working")], {
    triage: async () => ({ kind: "ok", detail: viewOf({ can_abandon: false, steps: stepsAt(3) }) }),
  })
  await openMenu()
  await screen.findByRole("menuitem", { name: /Copy triage ID/ })
  expect(screen.queryByRole("menuitem", { name: /Abandon Triage/ })).toBeNull()
})

test("confirming Abandon posts once and the page shows the empty state with the record in history", async () => {
  let abandoned = false
  const triage = async () => ({
    kind: "ok" as const,
    detail: abandoned
      ? viewOf({
          state: "abandoned",
          can_abandon: false,
          history: [past("t1", "2026-01-02T10:00:00Z", "abandoned")],
        })
      : viewOf({ can_abandon: true, revision: 7 }),
  })
  const abandonTriage = vi.fn(async () => ((abandoned = true), {}))
  const { emit } = render([session("working")], { triage, abandonTriage })
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: /Abandon Triage/ }))
  await screen.findByText(/Rows already written stay written/)
  fireEvent.click(screen.getByRole("button", { name: "Abandon" }))
  await waitFor(() => expect(abandonTriage).toHaveBeenCalledTimes(1))
  expect(abandonTriage).toHaveBeenCalledWith({ project: "gx", revision: 7 })
  emit("gx")
  await screen.findByText("No Triage for gx")
  expect(document.querySelector('[data-history-item="t1"]')?.textContent).toContain("abandoned")
  expect(abandonTriage).toHaveBeenCalledTimes(1)
})

test("Cancel posts nothing", async () => {
  const abandonTriage = vi.fn(async () => ({}))
  render([session("working")], {
    triage: async () => ({ kind: "ok", detail: viewOf({ can_abandon: true }) }),
    abandonTriage,
  })
  await openMenu()
  fireEvent.click(await screen.findByRole("menuitem", { name: /Abandon Triage/ }))
  fireEvent.click(await screen.findByRole("button", { name: "Cancel" }))
  await waitFor(() => expect(screen.queryByText(/Rows already written stay written/)).toBeNull())
  expect(abandonTriage).not.toHaveBeenCalled()
})

test("history lists past Triages in the order sent, read-only, with epic links", async () => {
  const result: Result = {
    ready_epic: "gx:ready-1",
    groups: [
      { action: "ready", items: [{ address: "gx:follow-ups/1", title: "a" }] },
      { action: "map", items: [{ address: "gx:follow-ups/2", title: "b", target: "gx:map-a/3" }] },
      { action: "close", items: [{ address: "gx:follow-ups/4", title: "c" }] },
    ],
  }
  const history = [
    past("new", "2026-02-01T10:00:00Z", "done", result),
    past("old", "2026-01-01T10:00:00Z", "abandoned"),
  ]
  render([session("working")], {
    triage: async () => ({ kind: "ok", detail: viewOf({ history }) }),
  })
  const region = await screen.findByRole("region", { name: "Past Triages" })
  const items = [...region.querySelectorAll("[data-history-item]")]
  expect(items.map((i) => i.getAttribute("data-history-item"))).toEqual(["new", "old"])
  expect(items[0].textContent).toContain("ready 1 · map 1 · merged 0 · closed 1 · kept 0")
  expect(items[0].textContent).toContain("done")
  expect(items[1].textContent).toContain("abandoned")
  expect(region.querySelector('a[href="#/e/gx:ready-1"]')).toBeTruthy()
  expect(region.querySelector('a[href="#/e/gx:map-a"]')).toBeTruthy()
  expect(region.querySelectorAll("button").length).toBe(0)
})

test("a done Triage with no ready epic shows no Queue epic button", async () => {
  const detail = viewOf({
    state: "done",
    steps: stepsAt(5),
    ring_percent: 100,
    result: { groups: [{ action: "close", items: [{ address: "gx:follow-ups/1", title: "a" }] }] },
  })
  render([], { triage: async () => ({ kind: "ok", detail }) })
  await screen.findByRole("button", { name: "More actions" })
  expect(screen.queryByRole("button", { name: "Queue epic" })).toBeNull()
})

test("a done Triage with a ready epic puts the Queue button on the ready line, not in the header", async () => {
  const detail = viewOf({
    state: "done",
    steps: stepsAt(5),
    ring_percent: 100,
    result: {
      ready_epic: "gx:ready-1",
      groups: [{ action: "ready", items: [{ address: "gx:follow-ups/1", title: "a" }] }],
    },
  })
  const ready = {
    address: "gx:ready-1",
    project: "gx",
    name: "ready-1",
    kind: "loop",
    state: "ready",
    done: 0,
    total: 1,
    queued: false,
    queueable: true,
    blocked_by_epics: [],
    addresses: ["gx:ready-1"],
  }
  render([], {
    triage: async () => ({ kind: "ok", detail }),
    epicList: async () => ({ epics: [ready], follow_ups: [] }) as never,
  })
  const queue = await screen.findByRole("button", { name: "Queue epic" })
  expect(screen.getByRole("region", { name: "Ready epic" }).contains(queue)).toBe(true)
  expect(screen.queryByRole("button", { name: "Continue" })).toBeNull()
})

test("a printed round shows the answered cards folded, the open card, the round header and the composer", async () => {
  const cards = [1, 2, 3, 4, 5].map((n) => ({
    label: String(n),
    title: `Title ${n}`,
    body: `Body ${n}?`,
    kind: "options",
    options: [
      { id: "a", label: "Alpha" },
      { id: "b", label: "Beta" },
    ],
  }))
  render([session("working")], {
    triage: async () => ({
      kind: "ok",
      detail: viewOf({
        steps: stepsAt(2),
        open_round: { number: 1, questions: 5, answered: 3 },
      }),
    }),
    chat: async () => ({
      kind: "ok",
      detail: {
        conversation_id: "c1",
        messages: [],
        questions: roundCards(cards as never),
        background: [],
      } as never,
    }),
  } as RouteFetchers)
  // A pick moves the card on, so each of the three picks is Alpha on the next question.
  for (let n = 1; n <= 3; n++) {
    await screen.findByText(`Question ${n} of 5`)
    fireEvent.click(screen.getByRole("button", { name: "Alpha" }))
  }
  await waitFor(() =>
    expect(document.querySelectorAll("[data-part=answered-question]").length).toBe(3),
  )
  expect(document.querySelector("[data-part=answered-question]")?.textContent).toContain(
    "Q1 Title 1 → Alpha",
  )
  expect(screen.getByText("Question 4 of 5")).toBeTruthy()
  expect(screen.getByText("Title 4")).toBeTruthy()
  expect(screen.getByText("Body 4?")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Add a note…" })).toBeTruthy()
  expect(screen.getByTestId("page-header-subtitle").textContent).toBe("Questions · round 1 of 3")
  expect(screen.getAllByText("Round 1 of 3 · 3 of 5 answered").length).toBeGreaterThan(0)
  const note = screen.getByRole("button", { name: "Note" }) as HTMLButtonElement
  expect(note.disabled).toBe(false)
  fireEvent.click(note)
  expect(await screen.findByPlaceholderText("A note on this round")).toBeTruthy()
})

test("the final approval round reads Final question, drops the round label and has no Approve button", async () => {
  const final = {
    label: "1",
    title: "Write this plan?",
    body: "39 ready, 14 to areas, 31 closed. Nothing changes until you say yes.",
    kind: "options",
    options: [
      { id: "yes", label: "Yes, write it" },
      { id: "no", label: "No, keep talking" },
    ],
  }
  render([session("working")], {
    triage: async () => ({
      kind: "ok",
      detail: viewOf({ steps: stepsAt(2), final_approval: true }),
    }),
    chat: async () => ({
      kind: "ok",
      detail: {
        conversation_id: "c1",
        messages: [],
        questions: roundCards([final] as never),
        background: [],
      } as never,
    }),
  } as RouteFetchers)
  expect(await screen.findByText("Final question")).toBeTruthy()
  expect(screen.queryByText(/Question 1 of 1/)).toBeNull()
  expect(screen.getByRole("button", { name: "Yes, write it" })).toBeTruthy()
  expect(screen.getByRole("button", { name: "No, keep talking" })).toBeTruthy()
  expect(screen.getByTestId("page-header-subtitle").textContent).toBe("Questions")
  expect(screen.queryByRole("button", { name: /Approve/ })).toBeNull()
})

const planRow = (n: number, action?: string, destination?: string) => ({
  ...row(n, "relevant"),
  action,
  destination,
})

const planView = (rows: ReturnType<typeof planRow>[]) =>
  viewOf({ steps: stepsAt(1), rows, ready_epic: "follow-ups-2026-11-14" })

const planRows = () => [
  ...[1, 2, 3, 4, 5].map((n) => planRow(n, "ready")),
  planRow(6, "area", "test-flakes"),
  planRow(7, "map", "ralph-loop"),
  planRow(8, "close", "fixed"),
  planRow(9, "close", "obsolete"),
  planRow(10, "merge", "gx:follow-ups/1"),
  planRow(11, "keep"),
]

test("after Route the Chat holds a read-only plan card with counts, areas and collapsed groups", async () => {
  render([session("working")], {
    triage: async () => ({ kind: "ok", detail: planView(planRows()) }),
  })
  const card = await screen.findByRole("region", { name: "Plan summary" })
  expect(
    within(screen.getByRole("region", { name: "Chat" })).getByRole("region", {
      name: "Plan summary",
    }),
  ).toBe(card)
  expect(card.textContent).toContain("Plan · 11 follow-ups")
  expect(card.textContent).toContain("read-only")
  expect(card.textContent).toContain("5Ready→ follow-ups-2026-11-14")
  expect(card.textContent).toContain("2To an area→ 2 areas")
  expect(card.textContent).toContain("2Closed1 fixed · 1 obsolete")
  expect(card.textContent).toContain("1Merged")
  expect(card.textContent).toContain("1Kept for later")
  expect(card.textContent).toContain("test-flakes1")
  expect(card.querySelector("a")).toBeNull()
  fireEvent.click(within(card).getByRole("button", { name: /5 Ready/ }))
  const group = await within(card).findByRole("group", { name: "Group: Ready 5" })
  expect(group.querySelectorAll("a").length).toBe(3)
  expect(group.querySelector("a")?.getAttribute("href")).toBe("#/t/gx:follow-ups/1")
  fireEvent.click(within(group).getByRole("button", { name: "2 more" }))
  expect(group.querySelectorAll("a").length).toBe(5)
})

test("the plan card re-renders its counts after a triage-changed event", async () => {
  let rows = planRows()
  const { emit } = render([session("working")], {
    triage: async () => ({ kind: "ok", detail: planView(rows) }),
  })
  const card = await screen.findByRole("region", { name: "Plan summary" })
  expect(card.textContent).toContain("5Ready")
  rows = rows.map((r, i) => (i === 0 ? { ...r, action: "keep" } : r))
  emit("gx")
  await waitFor(() => expect(card.textContent).toContain("4Ready"))
})

test("no plan card while a row has no plan yet", async () => {
  const rows = planRows()
  rows[0] = planRow(1)
  render([session("working")], {
    triage: async () => ({ kind: "ok", detail: planView(rows) }),
  })
  await screen.findByRole("heading", { level: 1 })
  expect(screen.queryByRole("region", { name: "Plan summary" })).toBeNull()
})
