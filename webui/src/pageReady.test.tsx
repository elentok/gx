import { cleanup, screen } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "./connection"
import { appRoutes, type RouteFetchers } from "./routes"
import { markReady } from "./routeSpans"
import { renderThemed } from "./ui/render"

vi.mock("./webPerf", () => ({ queueSample: vi.fn(), enableQueue: vi.fn(), dropQueue: vi.fn() }))
vi.mock("./routeSpans", async (orig) => ({ ...(await orig<object>()), markReady: vi.fn() }))

const ready = vi.mocked(markReady)

beforeEach(() => {
  sessionStorage.clear()
  ready.mockReset()
})
afterEach(cleanup)

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
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

// A fetcher that answers only when the test says so, so "not before" is observable.
function gate<T>(value: T) {
  let release!: () => void
  const wait = new Promise<void>((r) => (release = r))
  return { fetch: async () => (await wait, value), release }
}

const never = () => new Promise<never>(() => {})

// Every page that ends its span from its own primary fetch. The side fetchers never answer, so
// only the primary one can end the span.
const emptyEpics = { epics: [], follow_ups: [] }
const detail = {
  address: "gx:e/1",
  title: "Title one",
  status: "open",
  epic: "gx:e",
  map_epic: false,
  related: [],
  runs: [],
  answered: false,
  rounds: [],
  chat: [],
  actions: [],
}
const cases: {
  name: string
  path: string
  /** The primary fetcher, answered through the gate. */
  fetchers: (answer: () => Promise<unknown>) => RouteFetchers
  outcome: "ready" | "empty"
  /** Text that is on screen once the primary content has painted. */
  content: RegExp
}[] = [
  {
    name: "epics",
    path: "/epics",
    fetchers: (a) => ({ epicList: a as never, checksStatus: never as never }),
    outcome: "empty",
    content: /No epics/i,
  },
  {
    name: "archive",
    path: "/archive",
    fetchers: (a) => ({ archived: a as never, epicList: never }),
    outcome: "empty",
    content: /Nothing is archived/,
  },
  {
    name: "tips",
    path: "/tips",
    fetchers: (a) => ({ tips: a as never, epicList: never }),
    outcome: "empty",
    content: /No tips for this machine/,
  },
  {
    name: "schedules",
    path: "/schedules",
    fetchers: (a) => ({ schedules: a as never, epicList: never }),
    outcome: "empty",
    content: /No scheduled prompts yet/,
  },
  {
    name: "stats",
    path: "/stats",
    fetchers: (a) => ({ stats: a as never, epicList: never }),
    outcome: "ready",
    content: /Tickets merged/,
  },
  {
    name: "ticket",
    path: "/t/gx:e/1",
    fetchers: (a) => ({ ticket: a as never, epicList: never }),
    outcome: "ready",
    content: /Title one/,
  },
  {
    name: "epic",
    path: "/e/gx:e",
    fetchers: (a) => ({ epic: a as never, epicList: never }),
    outcome: "ready",
    content: /Ship the thing/,
  },
  {
    name: "cleanup",
    path: "/cleanup?project=gx",
    fetchers: (a) => ({ cleanup: a as never, epicList: never }),
    outcome: "ready",
    content: /Nothing to clean up/,
  },
]

const statsAnswer = {
  range: 7,
  days: ["2026-11-14"],
  throughput: {
    tickets_merged: [1],
    epics_merged: [0],
    designs_completed: [0],
    lines_added: [0],
    lines_removed: [0],
    follow_ups_created: [0],
    follow_ups_finished: [0],
    waiting_for_design: [0],
  },
  cost: { groups: [], sources: [], partial: false },
  friction: { pause_reasons: [], pauses_by_reason: [], run_events: [], pause_endings: [] },
  launch: { count: 0, target_ms: 15000, days: [], step_median_ms: {} },
  grilling: { words: [], questions: [] },
  mental_health: { break_starts: [0], break_unlocks: [0], bedtime_unlocks: [0] },
  limit_burn: {
    claude_5h: { days: [], sum: 0, average: 0 },
    claude_weekly: { days: [], sum: 0, average: 0 },
    codex_5h: { days: [], sum: 0, average: 0 },
    codex_weekly: { days: [], sum: 0, average: 0 },
  },
  web_performance: {
    hard_loads: [],
    route_changes: [],
    trend: [],
    slowest_routes: [],
    why_slow: [],
    poor_inp: { count: 0, top_actions: [] },
    dev_hidden: false,
  },
}

const answers: Record<string, unknown> = {
  epics: emptyEpics,
  archive: { epics: [] },
  tips: [],
  schedules: { refused: false, reason: "", message: "", schedules: [], projects: [] },
  stats: statsAnswer,
  ticket: { kind: "ok", detail },
  epic: {
    kind: "ok",
    detail: {
      address: "gx:e",
      kind: "loop",
      summary: "Ship the thing",
      done: 0,
      total: 0,
      agent_time: 0,
      cost_total: 0,
      rows: [],
      queueable: false,
      queued: false,
      stats: {},
    },
  },
  cleanup: { project: "gx", epics: [], worktrees: [], attic_refs: [] },
}

function renderPage(path: string, fetchers: RouteFetchers) {
  const router = createMemoryRouter(appRoutes(fetchers), { initialEntries: [path] })
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
}

test.each(cases)(
  "$name ends its span only after its primary content has painted",
  async ({ name, path, fetchers, outcome, content }) => {
    const g = gate(answers[name])
    // The content is checked inside markReady's call, so "after painted" is what is observed.
    let paintedAtReady: boolean | undefined
    ready.mockImplementation(() => {
      paintedAtReady = content.test(document.body.textContent ?? "")
    })
    renderPage(path, fetchers(g.fetch))
    await new Promise((r) => setTimeout(r, 0))
    expect(ready).not.toHaveBeenCalled()

    g.release()
    await screen.findByText(content)
    expect(ready).toHaveBeenCalledTimes(1)
    expect(ready.mock.calls[0]?.[0]).toBe(outcome)
    expect(paintedAtReady).toBe(true)
  },
)

test("a page whose primary fetch fails ends its span as an error", async () => {
  ready.mockImplementation(() => {})
  renderPage("/tips", { tips: async () => Promise.reject(new Error("boom")), epicList: never })
  await screen.findByRole("button", { name: "Retry" })
  expect(ready).toHaveBeenCalledTimes(1)
  expect(ready.mock.calls[0]?.[0]).toBe("error")
})
