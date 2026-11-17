import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { EpicList, StatsFriction } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import { STATS_REFETCH_DEBOUNCE_MS, type FetchStats, type Stats } from "./stats"

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
}

function eventConn() {
  const listeners = new Set<() => void>()
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: () => void) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
  } as unknown as Connection
  return { conn, emit: () => listeners.forEach((l) => l()) }
}

const days = ["2026-10-21", "2026-10-22", "2026-10-23"]

const statsOf = (over: Partial<Stats["throughput"]> = {}, extra: Partial<Stats> = {}): Stats => ({
  range: 30,
  days,
  throughput: {
    tickets_merged: [1, 2, 3],
    epics_merged: [0, 1, 0],
    designs_completed: [2, 0, 1],
    lines_added: [10, 20, 30],
    lines_removed: [1, 2, 3],
    follow_ups_created: [1, 0, 1],
    follow_ups_finished: [0, 2, 0],
    waiting_for_design: [4, 5, 7],
    ...over,
  },
  cost: { groups: [], sources: [], partial: false },
  friction: { pause_reasons: [], pauses_by_reason: [], run_events: [], pause_endings: [] },
  launch: { count: 0, target_ms: 15000, days: [], step_median_ms: {} },
  grilling: { words: [], questions: [] },
  limit_burn: {
    claude_5h: { days: [], sum: 0, average: 0 },
    claude_weekly: { days: [], sum: 0, average: 0 },
    codex_5h: { days: [], sum: 0, average: 0 },
    codex_weekly: { days: [], sum: 0, average: 0 },
  },
  mental_health: { break_starts: [0, 0, 0], break_unlocks: [0, 0, 0], bedtime_unlocks: [0, 0, 0] },
  web_performance: {
    hard_loads: [],
    route_changes: [],
    trend: [],
    slowest_routes: [],
    why_slow: [],
    poor_inp: { count: 0, top_actions: [] },
    dev_hidden: false,
  },
  ...extra,
})

const epicList = async (): Promise<EpicList> => ({
  epics: [
    {
      address: "gx:a",
      project: "gx",
      name: "a",
      kind: "loop",
      state: "ready",
      done: 0,
      total: 1,
      queued: false,
      queueable: true,
      blocked_by_epics: [],
      created_at: "2026-10-01T00:00:00Z",
    },
    {
      address: "web:b",
      project: "web",
      name: "b",
      kind: "loop",
      state: "ready",
      done: 0,
      total: 1,
      queued: false,
      queueable: true,
      blocked_by_epics: [],
      created_at: "2026-10-01T00:00:00Z",
    },
  ],
  follow_ups: [],
})

function renderStats(fetchStats: FetchStats, path = "/stats", conn: Connection = eventConn().conn) {
  const router = createMemoryRouter(appRoutes({ stats: fetchStats, epicList }), {
    initialEntries: [path],
  })
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

const cardText = async (title: string) => {
  const heading = await screen.findByText(title)
  let el: HTMLElement | null = heading
  while (el && !el.textContent?.includes("Legend") && !el.querySelector("[aria-label=Legend]"))
    el = el.parentElement
  return el?.textContent ?? ""
}

test("the page has a Stats heading whose subtitle names the project and range", async () => {
  renderStats(async () => statsOf(), "/stats?range=30&project=gx")
  expect(await screen.findByRole("heading", { level: 1, name: "Stats" })).toBeTruthy()
  expect(screen.getByTestId("page-header-subtitle").textContent).toBe("gx · last 30 days")
})

test("the Throughput cards follow their headline rules", async () => {
  renderStats(async () => statsOf())
  expect(await cardText("Tickets merged")).toContain("6")
  expect(await cardText("Tickets merged")).toContain("2.0/day")
  expect(await cardText("Epics merged")).toContain("1")
  expect(await cardText("Designs completed")).toContain("3")
  const loc = await cardText("Lines of code")
  expect(loc).toContain("+60 −6")
  expect(loc).not.toContain("/day")
  const followUps = await cardText("Follow-ups")
  expect(followUps).toContain("2 created · 2 finished")
  expect(followUps).toContain("0.7/day created · 0.7/day finished")
  const waiting = await cardText("Waiting for design")
  expect(waiting).toContain("7")
  expect(waiting).toContain("maps open now")
  expect(waiting).not.toContain("/day")
})

const cardEl = async (title: string) => {
  let el: HTMLElement | null = await screen.findByText(title)
  while (el && !el.querySelector("svg")) el = el.parentElement
  return el as HTMLElement
}

const amount = (value: number, no_data = false) => ({ value, no_data })

type Series = { tokens: ReturnType<typeof amount>[]; spend: ReturnType<typeof amount>[] }
const costOf = (groups: Record<string, Series>) => ({
  groups: Object.entries(groups).map(([group, v]) => ({ group, ...v, harness: [] })),
  sources: [],
  partial: false,
})

test("the Cost cards hatch a no-data day and say partial", async () => {
  const cost = costOf({
    Design: {
      tokens: [amount(100), amount(0, true), amount(300)],
      spend: [amount(1), amount(0, true), amount(2)],
    },
    Queue: {
      tokens: [amount(1000), amount(0, true), amount(2000)],
      spend: [amount(3), amount(0, true), amount(4.5)],
    },
  })
  renderStats(async () => statsOf({}, { cost }))
  const tokens = await cardText("Tokens")
  expect(tokens).toContain("3K")
  expect(tokens).toContain("partial")
  expect((await cardEl("Tokens")).querySelectorAll("[data-nodata]")).toHaveLength(1)
  const spend = await cardText("Spend (est.)")
  expect(spend).toContain("$11")
  expect(spend).toContain("Codex counts $0")
  expect(spend).toContain("Design")
  expect(spend).toContain("Queue")
})

test("a stamp day where only one group is no-data is hatched whole", async () => {
  const zero = [amount(0), amount(0)]
  const cost = costOf({
    Queue: { tokens: [amount(5), amount(0, true)], spend: [amount(1), amount(0, true)] },
    Design: { tokens: zero, spend: zero },
    "One-off": { tokens: zero, spend: zero },
    Other: { tokens: zero, spend: zero },
  })
  renderStats(async () => statsOf({}, { cost }))
  const card = await cardEl("Tokens")
  expect(card.querySelectorAll("[data-nodata]")).toHaveLength(1)
  expect(card.querySelectorAll("[data-nodata='1']")).toHaveLength(1)
})

test("Cost cards with data on every day are not partial", async () => {
  const full = [amount(1), amount(1), amount(1)]
  renderStats(async () =>
    statsOf(
      {},
      {
        cost: costOf({
          Design: { tokens: full, spend: full },
          Queue: { tokens: full, spend: full },
        }),
      },
    ),
  )
  expect(await cardText("Tokens")).not.toContain("partial")
})

test("the Cost cards draw rebuilt and stamp days lighter, say partial and split Claude and Codex in the tooltip", async () => {
  const claudeCodex = { claude: { tokens: 700, dollars: 2 }, codex: { tokens: 300, dollars: 0 } }
  const cost = {
    groups: [
      {
        group: "Queue",
        tokens: [amount(1000), amount(500), amount(0, true)],
        spend: [amount(2), amount(1), amount(0, true)],
        harness: [claudeCodex, {}, {}],
      },
      {
        group: "Design",
        tokens: [amount(0), amount(0), amount(0, true)],
        spend: [amount(0), amount(0), amount(0, true)],
        harness: [{}, {}, {}],
      },
    ],
    sources: ["live", "rebuilt", "stamp"],
    partial: true,
  }
  renderStats(async () => statsOf({}, { cost }))
  expect(await cardText("Tokens")).toContain("partial")
  const card = await cardEl("Tokens")
  expect(card.querySelectorAll("[data-style='dim']").length).toBeGreaterThan(0)
  expect(card.querySelectorAll("[data-bar='0'][data-style='dim']")).toHaveLength(0)
  expect(card.querySelectorAll("[data-nodata]")).toHaveLength(1)
  fireEvent.mouseEnter(card.querySelector("[data-hit='0']") as Element)
  expect(within(card).getByRole("tooltip").textContent).toContain("Claude: 700")
  expect(within(card).getByRole("tooltip").textContent).toContain("Codex: 300")
  fireEvent.mouseEnter(card.querySelector("[data-hit='1']") as Element)
  expect(within(card).getByRole("tooltip").textContent).toContain("rebuilt from transcripts")
  fireEvent.mouseEnter(card.querySelector("[data-hit='2']") as Element)
  expect(within(card).getByRole("tooltip").textContent).toContain("ticket stamps only")
  const spend = await cardEl("Spend (est.)")
  fireEvent.mouseEnter(spend.querySelector("[data-hit='0']") as Element)
  expect(within(spend).getByRole("tooltip").textContent).toContain("Claude: $2")
})

// Replaces e2e flow 183: the legend follows the server's group order, and a quiet live day is a zero bar.
test("the Cost legend lists the groups in order and a quiet live day is a zero bar, not hatched", async () => {
  const live = [amount(900), amount(0)]
  const quiet = [amount(0), amount(0)]
  const cost = costOf({
    Queue: { tokens: live, spend: live },
    Design: { tokens: quiet, spend: quiet },
    "One-off": { tokens: quiet, spend: quiet },
    Other: { tokens: quiet, spend: quiet },
  })
  renderStats(async () => statsOf({}, { cost }))
  for (const title of ["Tokens", "Spend (est.)"]) {
    const card = await cardEl(title)
    expect(within(card).getByLabelText("Legend").textContent).toBe("QueueDesignOne-offOther")
  }
  const tokens = await cardEl("Tokens")
  expect(tokens.querySelectorAll("[data-nodata]")).toHaveLength(0)
  fireEvent.mouseEnter(tokens.querySelector("[data-hit='1']") as Element)
  const tip = within(tokens).getByRole("tooltip").textContent ?? ""
  expect(tip).toContain("Queue: 0")
  expect(tip).not.toContain("no data")
})

test("the Grilling section sits between Cost and effort and Friction", async () => {
  renderStats(async () => statsOf())
  await screen.findByText("Words read")
  // The toaster's own container is a region too; only the sections name themselves.
  const names = screen
    .getAllByRole("region")
    .map((r) => r.getAttribute("aria-labelledby"))
    .filter((n) => n !== null)
  expect(names).toEqual([
    "stats-throughput",
    "stats-cost",
    "stats-grilling",
    "stats-friction",
    "stats-launch",
    "stats-web-performance",
    "stats-mental-health",
  ])
  expect(screen.getByText("Questions asked")).toBeTruthy()
})

test("the Mental health card totals Breaks and unlocks and splits unlocks in the legend", async () => {
  const mh = { break_starts: [2, 3, 1], break_unlocks: [1, 0, 1], bedtime_unlocks: [0, 2, 0] }
  renderStats(async () => statsOf({}, { mental_health: mh }))
  const text = await cardText("Breaks and unlocks")
  expect(text).toContain("6 Breaks · 4 unlocks")
  expect(text).toContain("2.0/day Breaks · 1.3/day unlocks · 2 Break, 2 Bedtime")
  expect(text).toContain("ignores the project filter")
  expect(text).toContain("Break unlocks")
  expect(text).toContain("Bedtime unlocks")
})

test("the Mental health card ignores the project filter", async () => {
  const mh = { break_starts: [1, 1, 1], break_unlocks: [0, 0, 0], bedtime_unlocks: [0, 0, 0] }
  renderStats(async () => statsOf({}, { mental_health: mh }), "/stats?project=gx")
  expect(await cardText("Breaks and unlocks")).toContain("3 Breaks · 0 unlocks")
})

test("the Grilling cards skip no-data days in the total and average and hatch them", async () => {
  const grilling = {
    words: [amount(1000), amount(0, true), amount(2500)],
    questions: [amount(4), amount(0, true), amount(6)],
  }
  renderStats(async () => statsOf({}, { grilling }))
  const words = await cardText("Words read")
  expect(words).toContain("3,500")
  expect(words).toContain("1,750/day")
  expect(words).toContain("partial")
  expect((await cardEl("Words read")).querySelectorAll("[data-nodata]")).toHaveLength(1)
  const questions = await cardText("Questions asked")
  expect(questions).toContain("10")
  expect(questions).toContain("5/day")
  expect((await cardEl("Questions asked")).querySelectorAll("[data-nodata]")).toHaveLength(1)
})

test("the Pauses tooltip lists a reason folded into other", async () => {
  const friction: StatsFriction = {
    pause_reasons: ["question", "other"],
    pauses_by_reason: [{ question: 2, stalled: 1 }, {}, { question: 1 }],
    run_events: [{}, {}, {}],
    pause_endings: [{}, {}, {}],
  }
  renderStats(async () => statsOf({}, { friction }))
  const card = await cardEl("Pauses")
  expect(card.textContent).toContain("other")
  fireEvent.mouseEnter(card.querySelector("[data-hit='0']") as Element)
  const tip = within(screen.getByRole("tooltip"))
  expect(tip.getByText("question: 2")).toBeTruthy()
  expect(tip.getByText("other: 1")).toBeTruthy()
  expect(tip.getByText("stalled: 1")).toBeTruthy()
  expect(card.querySelectorAll("[data-bar][data-series='stalled']")).toHaveLength(0)
})

test("Pause endings headline is the web share with the median on the small line", async () => {
  const friction: StatsFriction = {
    pause_reasons: [],
    pauses_by_reason: [{}, {}, {}],
    run_events: [{}, {}, {}],
    pause_endings: [{ web: 5, tui: 2, gx: 4 }, {}, { web: 0, cli: 1 }],
    web_share: 0.625,
    median_resume_secs: 270,
  }
  renderStats(async () => statsOf({}, { friction }))
  const ending = await cardText("Pause endings")
  expect(ending).toContain("63% web")
  expect(ending).toContain("median 5m to resume")
  expect(ending).not.toContain("partial")
})

test("the Launch section shows the median, share under target, per-day chart and step medians", async () => {
  const launch = {
    count: 4,
    median_ms: 12500,
    share_under_target: 0.75,
    target_ms: 15000,
    days: [amount(12000), amount(0, true), amount(13000)],
    step_median_ms: { worktree: 800, agent: 9000 },
  }
  renderStats(async () => statsOf({}, { launch }))
  const card = await cardText("Launch time")
  expect(card).toContain("12.5s")
  expect(card).toContain("75% under 15.0s")
  expect((await cardEl("Launch time")).querySelectorAll("[data-nodata]")).toHaveLength(1)
  const steps = (await screen.findByTestId("launch-steps")).textContent ?? ""
  expect(steps).toContain("agent9.0s")
  expect(steps).toContain("worktree0.8s")
})

test("the Launch section shows a dash with no launches", async () => {
  renderStats(async () => statsOf())
  const card = await cardText("Launch time")
  expect(card).toContain("—")
  expect(card).not.toContain("0.0s")
  expect((await screen.findByTestId("launch-steps")).textContent).toContain("—")
})

test("the URL filters drive the fetch and the controls", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => statsOf())
  renderStats(fetchStats, "/stats?range=7&project=gx")
  await screen.findByText("Tickets merged")
  expect(fetchStats).toHaveBeenCalledWith({ range: 7, project: "gx", includeDev: false })
  expect((screen.getByLabelText("Range") as HTMLSelectElement).value).toBe("7")
  await waitFor(() =>
    expect(
      within(screen.getByLabelText("Project"))
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["All projects", "gx", "web"]),
  )
  expect((screen.getByLabelText("Project") as HTMLSelectElement).value).toBe("gx")
})

test("changing a control rewrites the URL and refetches", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => statsOf())
  const router = renderStats(fetchStats)
  await screen.findByText("Tickets merged")
  expect(fetchStats).toHaveBeenLastCalledWith({ range: 7, project: "", includeDev: false })
  fireEvent.change(screen.getByLabelText("Range"), { target: { value: "90" } })
  await waitFor(() =>
    expect(fetchStats).toHaveBeenLastCalledWith({ range: 90, project: "", includeDev: false }),
  )
  expect(router.state.location.search).toBe("?range=90")
})

const perfCard = (
  metric: string,
  count: number,
  over: Partial<Stats["web_performance"]["hard_loads"][number]> = {},
) => ({ metric, count, has_threshold: ["lcp", "inp", "cls", "fcp"].includes(metric), ...over })

const perfStats = (wp: Partial<Stats["web_performance"]> = {}) =>
  statsOf({}, { web_performance: { ...statsOf().web_performance, ...wp } })

test("the Web performance cards show value, pill, the server's change and the definition", async () => {
  renderStats(async () =>
    perfStats({
      hard_loads: [
        perfCard("lcp", 412, { p75: 2310, status: "good", change_pct: -7.2 }),
        perfCard("inp", 96, { p75: 168, status: "needs-improvement" }),
        perfCard("cls", 412, { p75: 0.04, status: "poor", change_pct: 0 }),
        perfCard("app_ready", 412, { p75: 1840 }),
        perfCard("fcp", 14, { p75: 1120, status: "good", change_pct: -11 }),
      ],
    }),
  )
  const lcp = (await screen.findByTestId("perf-card-lcp")).textContent ?? ""
  expect(lcp).toContain("2.31 s")
  expect(lcp).toContain("412 samples")
  expect(lcp).toContain("good")
  expect(lcp).toContain("-7% vs previous 7d")
  expect(lcp).toContain("Largest Contentful Paint")
  const inp = screen.getByTestId("perf-card-inp").textContent ?? ""
  expect(inp).toContain("168 ms")
  expect(inp).toContain("needs improvement")
  expect(inp).not.toContain("%")
  expect(inp).toContain("—")
  const cls = screen.getByTestId("perf-card-cls").textContent ?? ""
  expect(cls).toContain("0.04")
  expect(cls).toContain("poor")
  expect(cls).toContain("0% vs previous 7d")
  const app = screen.getByTestId("perf-card-app_ready").textContent ?? ""
  expect(app).toContain("no standard threshold")
  expect(app).not.toContain("good")
  const fcp = screen.getByTestId("perf-card-fcp")
  expect(fcp.textContent).toContain("not enough samples")
  expect(fcp.textContent).not.toContain("good")
  expect(fcp.dataset.low).toBe("true")
})

test("a card's has_threshold flag decides whether it says there is no standard threshold", async () => {
  renderStats(async () =>
    perfStats({
      hard_loads: [
        perfCard("lcp", 412, { p75: 2310, has_threshold: false }),
        perfCard("app_ready", 412, { p75: 1840, has_threshold: true }),
      ],
    }),
  )
  expect((await screen.findByTestId("perf-card-lcp")).textContent).toContain(
    "no standard threshold",
  )
  expect(screen.getByTestId("perf-card-app_ready").textContent).not.toContain(
    "no standard threshold",
  )
})

test("a project filter leaves the Web performance values unchanged and the scope text says so", async () => {
  const wp = perfStats({
    hard_loads: [perfCard("lcp", 412, { p75: 2310, status: "good", change_pct: -7.2 })],
  }).web_performance
  // Like the server: the project narrows the other sections, never Web performance.
  const fetchStats = vi.fn<FetchStats>(async ({ project }) =>
    statsOf({ tickets_merged: project ? [9, 9, 9] : [1, 2, 3] }, { web_performance: wp }),
  )
  const sectionText = async () => (await screen.findByTestId("perf-card-lcp")).textContent ?? ""

  renderStats(fetchStats, "/stats")
  const unfiltered = await sectionText()
  expect(await cardText("Tickets merged")).toContain("6")
  cleanup()
  sessionStorage.clear()

  renderStats(fetchStats, "/stats?project=gx")
  expect(await sectionText()).toBe(unfiltered)
  expect(unfiltered).toContain("2.31 s")
  expect(await cardText("Tickets merged")).toContain("27")
  expect(fetchStats).toHaveBeenLastCalledWith({ range: 7, project: "gx", includeDev: false })
  expect(screen.getByText(/All projects, ignores the project filter/)).toBeTruthy()
})

test("the Route changes tab shows its cards and round-trips through the URL", async () => {
  const router = renderStats(async () =>
    perfStats({
      hard_loads: [perfCard("lcp", 30, { p75: 2000, status: "good" })],
      route_changes: [perfCard("route_ready", 50, { p75: 420 })],
    }),
  )
  await screen.findByTestId("perf-card-lcp")
  expect(screen.queryByTestId("perf-card-route_ready")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "Route changes" }))
  expect(await screen.findByTestId("perf-card-route_ready")).toBeTruthy()
  expect(router.state.location.search).toBe("?perf=route")
  fireEvent.click(screen.getByRole("button", { name: "Hard loads" }))
  await screen.findByTestId("perf-card-lcp")
  expect(router.state.location.search).toBe("")
})

test("Include dev refetches with includeDev and lives in the URL", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => perfStats())
  const router = renderStats(fetchStats)
  await screen.findByText("Web performance")
  const toggle = screen.getByLabelText("Include dev") as HTMLInputElement
  expect(toggle.checked).toBe(false)
  fireEvent.click(toggle)
  await waitFor(() =>
    expect(fetchStats).toHaveBeenLastCalledWith({ range: 7, project: "", includeDev: true }),
  )
  expect(router.state.location.search).toBe("?dev=1")
  expect((screen.getByLabelText("Include dev") as HTMLInputElement).checked).toBe(true)
})

// A filter change moves to a new query id, and the page cache holds only the old id's entry,
// so the new id has no cache. The fetch for it is held by the test.
function heldSecondFetch() {
  let release: (s: Stats) => void = () => {}
  let fail: (e: Error) => void = () => {}
  const fetchStats = vi.fn<FetchStats>().mockResolvedValueOnce(perfStats())
  fetchStats.mockImplementationOnce(
    () =>
      new Promise<Stats>((res, rej) => {
        release = res
        fail = rej
      }),
  )
  return { fetchStats, release: (s: Stats) => release(s), fail: (e: Error) => fail(e) }
}

test("Include dev keeps the same checkbox mounted while the new numbers load (story 39a)", async () => {
  const { fetchStats, release } = heldSecondFetch()
  renderStats(fetchStats)
  await screen.findByText("Web performance")
  const toggle = screen.getByLabelText("Include dev") as HTMLInputElement
  fireEvent.click(toggle)
  await waitFor(() => expect(fetchStats).toHaveBeenCalledTimes(2))
  expect(toggle.isConnected).toBe(true)
  expect(screen.queryByRole("status", { name: "Loading stats" })).toBeNull()
  const body = screen.getByText("Web performance").closest("[aria-busy]")
  expect(body?.getAttribute("aria-busy")).toBe("true")
  release(perfStats())
  await waitFor(() => expect(body?.getAttribute("aria-busy")).toBe("false"))
  expect(screen.getByLabelText("Include dev")).toBe(toggle)
})

test("a range or project change keeps the previous cards while the new fetch loads", async () => {
  const { fetchStats, release } = heldSecondFetch()
  const router = renderStats(fetchStats)
  await screen.findByText("Tickets merged")
  await router.navigate("/stats?range=30&project=gx")
  await waitFor(() => expect(fetchStats).toHaveBeenCalledTimes(2))
  expect(screen.getByText("Tickets merged")).toBeTruthy()
  expect(screen.queryByRole("status", { name: "Loading stats" })).toBeNull()
  release(perfStats())
  await waitFor(() =>
    expect(
      screen.getByText("Tickets merged").closest("[aria-busy]")?.getAttribute("aria-busy"),
    ).toBe("false"),
  )
})

test("a failed filter-change refetch keeps the stale numbers, with no loader or error", async () => {
  const { fetchStats, fail } = heldSecondFetch()
  const router = renderStats(fetchStats)
  await screen.findByText("Tickets merged")
  await router.navigate("/stats?range=30")
  await waitFor(() => expect(fetchStats).toHaveBeenCalledTimes(2))
  fail(new Error("boom"))
  await act(async () => {})
  expect(screen.getByText("Tickets merged")).toBeTruthy()
  expect(screen.queryByRole("status", { name: "Loading stats" })).toBeNull()
  expect(screen.queryByText("Could not load stats")).toBeNull()
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull()
})

test("with no samples the section shows the empty state, and says dev samples are hidden", async () => {
  renderStats(async () => perfStats({ dev_hidden: true }))
  const empty = (await screen.findByTestId("perf-empty")).textContent ?? ""
  expect(empty).toContain("No web performance samples in this range.")
  expect(empty).toContain("Dev samples are hidden.")
  expect(screen.getByLabelText("Include dev")).toBeTruthy()
})

test("the empty state leaves out the dev line when no dev samples are hidden", async () => {
  renderStats(async () => perfStats())
  const empty = (await screen.findByTestId("perf-empty")).textContent ?? ""
  expect(empty).not.toContain("Dev samples")
})

const trendStats = () =>
  statsOf(
    {},
    {
      days: ["2026-11-12", "2026-11-13", "2026-11-14"],
      web_performance: {
        ...statsOf().web_performance,
        hard_loads: [perfCard("lcp", 30, { p75: 2000, status: "good" })],
        route_changes: [perfCard("route_ready", 50, { p75: 420 })],
        trend: [
          {
            metric: "lcp",
            days: [{ count: 5, p75: 2100 }, { count: 0 }, { count: 40, p75: 2300 }],
          },
          {
            metric: "cls",
            days: [{ count: 30, p75: 0.04 }, { count: 30, p75: 0.05 }, { count: 30 }],
          },
        ],
        slowest_routes: [
          { route: "ticket", p75: 780, count: 141 },
          { route: "epic", p75: 640, count: 212 },
          { route: "stats", p75: 610, count: 6 },
        ],
      },
    },
  )

test("the trend shades low-sample days, skips empty days and ignores the tab", async () => {
  renderStats(async () => trendStats())
  const svg = await screen.findByRole("img", { name: "daily p75 LCP" })
  expect(svg.textContent).toContain("fewer than 20 samples")
  expect(svg.querySelectorAll("[data-testid=trend-low-day]").length).toBe(2)
  expect(svg.querySelectorAll("[data-testid=trend-point]").length).toBe(2)
  fireEvent.click(screen.getByRole("button", { name: "Route changes" }))
  await screen.findByTestId("perf-card-route_ready")
  expect(screen.getByRole("img", { name: "daily p75 LCP" })).toBeTruthy()
})

test("the trend picker shows one metric with its own unit and is not in the URL", async () => {
  const router = renderStats(async () => trendStats())
  expect((await screen.findByRole("img", { name: "daily p75 LCP" })).textContent).toContain(" s")
  fireEvent.click(screen.getByRole("button", { name: "CLS" }))
  const cls = screen.getByRole("img", { name: "daily p75 CLS" })
  expect(cls.textContent).not.toMatch(/ms| s$/)
  expect(cls.textContent).toContain("0.05")
  expect(router.state.location.search).toBe("")
  for (const l of ["LCP", "INP", "CLS", "FCP", "App ready", "Route ready"]) {
    expect(screen.getByRole("button", { name: l })).toBeTruthy()
  }
})

test("the slowest routes table keeps the server's order and dims routes under 20 samples", async () => {
  renderStats(async () => trendStats())
  const table = await screen.findByTestId("slowest-routes")
  expect([...table.querySelectorAll("tbody tr")].map((r) => r.getAttribute("data-testid"))).toEqual(
    ["route-row-ticket", "route-row-epic", "route-row-stats"],
  )
  const stats = screen.getByTestId("route-row-stats")
  expect(stats.getAttribute("data-low")).toBe("true")
  expect(stats.textContent).toContain("6 · not enough")
  expect(screen.getByTestId("route-row-ticket").getAttribute("data-low")).toBeNull()
})

test("with no slowest routes the section shows a dash and no table", async () => {
  const s = trendStats()
  renderStats(async () => ({ ...s, web_performance: { ...s.web_performance, slowest_routes: [] } }))
  const box = await screen.findByTestId("slowest-routes")
  expect(box.querySelector("table")).toBeNull()
  expect(box.textContent).toContain("—")
})

test("a bad range falls back to 7 days", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => statsOf())
  renderStats(fetchStats, "/stats?range=5")
  await screen.findByText("Tickets merged")
  expect(fetchStats).toHaveBeenCalledWith({ range: 7, project: "", includeDev: false })
})

test("shows the spinner first, then an error with Retry when the fetch fails", async () => {
  const fetchStats = vi
    .fn<FetchStats>()
    .mockRejectedValueOnce(new Error("boom"))
    .mockResolvedValue(statsOf())
  renderStats(fetchStats)
  expect(screen.getByRole("status", { name: "Loading stats" })).toBeTruthy()
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }))
  await screen.findByText("Tickets merged")
})

test("repaints from the page cache before the fetch lands", async () => {
  renderStats(async () => statsOf())
  await screen.findByText("Tickets merged")
  cleanup()
  renderStats(() => new Promise<Stats>(() => {}))
  expect(screen.queryByRole("status", { name: "Loading stats" })).toBeNull()
  expect(screen.getByText("Tickets merged")).toBeTruthy()
})

test("a server event triggers one debounced refetch", async () => {
  vi.useFakeTimers()
  const { conn, emit } = eventConn()
  const fetchStats = vi.fn<FetchStats>(async () => statsOf())
  renderStats(fetchStats, "/stats", conn)
  await act(async () => {})
  const before = fetchStats.mock.calls.length
  act(() => {
    emit()
    emit()
  })
  await act(async () => {
    vi.advanceTimersByTime(STATS_REFETCH_DEBOUNCE_MS - 1)
  })
  expect(fetchStats).toHaveBeenCalledTimes(before)
  await act(async () => {
    vi.advanceTimersByTime(1)
  })
  expect(fetchStats).toHaveBeenCalledTimes(before + 1)
})

const buildStats = (wp: Partial<Stats["web_performance"]> = {}) =>
  perfStats({
    hard_loads: [perfCard("lcp", 30, { p75: 2000, status: "good" })],
    build_strip: {
      current: { build_id: "b-a91cdead", web_version: "0.14.2" },
      previous: { build_id: "b-5be0beef", web_version: "0.14.1" },
      lcp_change_pct: -12.4,
      inp_change_pct: undefined,
      js_change_pct: -31,
    },
    ...wp,
  })

test("the build strip names both builds and shows the changes, a null change as a dash", async () => {
  renderStats(async () => buildStats())
  const strip = (await screen.findByTestId("perf-build-strip")).textContent ?? ""
  expect(strip).toContain("Current build0.14.2 · a91c")
  expect(strip).toContain("Compared with0.14.1 · 5be0")
  expect(strip).toContain("LCP p75 -12% · INP — · JS transfer -31%")
})

test("with no previous build the strip shows only the current build", async () => {
  renderStats(async () =>
    buildStats({ build_strip: { current: { build_id: "b-a91cdead", web_version: "0.14.2" } } }),
  )
  const strip = (await screen.findByTestId("perf-build-strip")).textContent ?? ""
  expect(strip).toContain("0.14.2 · a91c")
  expect(strip).toContain("no earlier build in range")
  expect(strip).not.toContain("Compared with")
  expect(strip).not.toContain("%")
})

test("Why slow? is collapsed, then shows rows that dim under 20 samples and dash per the rule", async () => {
  renderStats(async () =>
    buildStats({
      why_slow: [
        { metric: "ttfb", count: 50, p75: 210, previous_p75: 240, change_pct: -12 },
        { metric: "api_latency", count: 5, p75: 320, previous_p75: 390 },
        { metric: "js_bytes", count: 50, p75: 286000, previous_p75: 412000, change_pct: -31 },
        { metric: "css_bytes", count: 50, p75: 41000 },
      ],
      poor_inp: {
        count: 40,
        input_delay_p75: 30,
        processing_p75: 120,
        presentation_delay_p75: 45,
        top_actions: [
          { action: "click:merge", count: 9 },
          { action: "key:j", count: 4 },
        ],
        worst_chunk: "app.js",
      },
    }),
  )
  await screen.findByTestId("perf-build-strip")
  expect(screen.queryByTestId("why-row-ttfb")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: /Why slow\? Diagnostics/ }))
  const ttfb = screen.getByTestId("why-row-ttfb")
  expect(ttfb.textContent).toBe("TTFB210 ms240 ms")
  expect(ttfb.dataset.low).toBeUndefined()
  const api = screen.getByTestId("why-row-api_latency")
  expect(api.textContent).toBe("Blocking API latencynot enough—")
  expect(api.dataset.low).toBe("true")
  expect(screen.getByTestId("why-row-js_bytes").textContent).toContain("286 KB412 KB")
  expect(screen.getByTestId("why-row-css_bytes").textContent).toBe("CSS transferred41.0 KB—")
  const poor = screen.getByTestId("perf-poor-inp").textContent ?? ""
  expect(poor).toContain("(40 samples)")
  expect(poor).toContain("Processing120 ms")
  expect(poor).toContain("Slow action: click:merge9 poor")
  expect(poor).toContain("Worst-frame chunkapp.js")
})

test("JSON nulls from the server render as dashes, not 0%", async () => {
  const nulls = (v: object) => v as never
  renderStats(async () =>
    perfStats({
      hard_loads: [nulls(perfCard("lcp", 9, { p75: 3840, change_pct: null as never }))],
      build_strip: nulls({
        current: { build_id: "b-a91cdead", web_version: "1.0.0" },
        previous: null,
        lcp_change_pct: null,
        inp_change_pct: null,
        js_change_pct: null,
      }),
      trend: nulls([
        {
          metric: "lcp",
          days: [
            { count: 0, p75: null },
            { count: 5, p75: 3800 },
          ],
        },
      ]),
    }),
  )
  const card = await screen.findByTestId("perf-card-lcp")
  expect(card.textContent).not.toContain("%")
  expect(card.textContent).toContain("—")
  const strip = screen.getByTestId("perf-build-strip").textContent ?? ""
  expect(strip).toContain("no earlier build in range")
  expect(strip).not.toContain("%")
  expect(screen.getAllByTestId("trend-point").length).toBe(1)
})

test("the trend picker goes back to LCP on a new render", async () => {
  renderStats(async () => trendStats())
  await screen.findByRole("img", { name: "daily p75 LCP" })
  fireEvent.click(screen.getByRole("button", { name: "CLS" }))
  expect(screen.getByRole("img", { name: "daily p75 CLS" })).toBeTruthy()
  cleanup()
  renderStats(async () => trendStats())
  expect(await screen.findByRole("img", { name: "daily p75 LCP" })).toBeTruthy()
})

test("the poor-INP block dims with not enough below 20 poor samples", async () => {
  renderStats(async () =>
    buildStats({ poor_inp: { count: 6, top_actions: [], processing_p75: 100 } }),
  )
  await screen.findByTestId("perf-build-strip")
  fireEvent.click(screen.getByRole("button", { name: /Why slow\? Diagnostics/ }))
  const poor = screen.getByTestId("perf-poor-inp")
  expect(poor.dataset.low).toBe("true")
  expect(poor.textContent).toContain("not enough")
  expect(poor.textContent).not.toContain("Processing")
})
