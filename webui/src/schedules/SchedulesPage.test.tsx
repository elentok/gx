import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { ScheduleInfo, ScheduleShowResult, SchedulesResult } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { PostSwitch } from "./schedules"

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const state = {
  snapshot: undefined,
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
} as unknown as ConnectionState
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection

const sched = (over: Partial<ScheduleInfo>): ScheduleInfo => ({
  name: "Morning",
  slug: "morning",
  project: "gx",
  days: ["mon", "tue", "wed", "thu", "fri"],
  times: ["08:30"],
  harness: "",
  tier: "",
  default_harness: "claude",
  late_limit_hours: 2,
  switch: true,
  project_found: true,
  next_run: new Date(Date.now() + 3 * 86_400_000).toISOString(),
  updated_at: "",
  ...over,
})

const list = (schedules: ScheduleInfo[]): SchedulesResult => ({
  refused: false,
  reason: "",
  message: "",
  schedules,
  projects: [],
})

function renderPage(schedules: ScheduleInfo[], post?: PostSwitch) {
  sessionStorage.clear()
  const router = createMemoryRouter(
    appRoutes({
      schedules: async () => list(schedules),
      switchSchedule: post,
      epicList: async () => ({ epics: [], follow_ups: [] }),
    }),
    { initialEntries: ["/schedules"] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

test("rows show name, days, times and the next run", async () => {
  renderPage([sched({ name: "Morning" })])
  expect(await screen.findByText("Morning")).toBeTruthy()
  expect(screen.getByText("Weekdays · 08:30")).toBeTruthy()
  expect(screen.getByText(/^Next /)).toBeTruthy()
})

test("a Schedule with the switch off reads Off here and no next run", async () => {
  renderPage([sched({ switch: false, next_run: "" })])
  expect(await screen.findByText("Off here")).toBeTruthy()
  expect(screen.queryByText(/^Next /)).toBeNull()
  expect(
    screen.getByRole("switch", { name: "Run on this machine" }).getAttribute("aria-checked"),
  ).toBe("false")
})

test("clicking the switch writes and the row updates without opening the row", async () => {
  const next = new Date(Date.now() + 86_400_000).toISOString()
  const post = vi.fn<PostSwitch>(
    async (project, slug, on) =>
      ({
        refused: false,
        reason: "",
        message: "",
        schedule: sched({ project, slug, switch: on, next_run: next }),
        prompt: "",
        last_runs: [],
      }) as ScheduleShowResult,
  )
  renderPage([sched({ switch: false, next_run: "" })], post)
  const sw = await screen.findByRole("switch", { name: "Run on this machine" })
  fireEvent.click(sw)
  await waitFor(() => expect(screen.queryByText("Off here")).toBeNull())
  expect(post).toHaveBeenCalledWith("gx", "morning", true)
  expect(screen.getByText(/^Next Tomorrow/)).toBeTruthy()
  expect(screen.queryByText("Not built yet")).toBeNull()
})

const BANNER = /^Every schedule is off on this machine/

const answerOf = (project: string, slug: string, on: boolean) =>
  ({
    refused: false,
    reason: "",
    message: "",
    schedule: sched({ project, slug, switch: on, next_run: on ? sched({}).next_run : "" }),
    prompt: "",
    last_runs: [],
  }) as ScheduleShowResult

test("an empty list shows the example text, New and no banner", async () => {
  renderPage([])
  expect(await screen.findByText(/^No scheduled prompts yet\./)).toBeTruthy()
  expect(screen.getByText(/for example every day at 07:00 and 21:00/)).toBeTruthy()
  expect(screen.getByRole("button", { name: "New" })).toBeTruthy()
  expect(screen.queryByText(BANNER)).toBeNull()
})

test("the banner shows when every switch is off", async () => {
  renderPage([sched({ switch: false, next_run: "" })])
  expect(await screen.findByText(BANNER)).toBeTruthy()
})

test("the banner is hidden while one switch is on", async () => {
  renderPage([sched({}), sched({ slug: "b", name: "B", switch: false, next_run: "" })])
  await screen.findByText("Morning")
  expect(screen.queryByText(BANNER)).toBeNull()
})

test("turning the last switch off shows the banner and turning one on hides it", async () => {
  const post = vi.fn<PostSwitch>(async (p, s, on) => answerOf(p, s, on))
  renderPage([sched({})], post)
  const sw = await screen.findByRole("switch", { name: "Run on this machine" })
  expect(screen.queryByText(BANNER)).toBeNull()
  fireEvent.click(sw)
  expect(await screen.findByText(BANNER)).toBeTruthy()
  fireEvent.click(screen.getByRole("switch", { name: "Run on this machine" }))
  await waitFor(() => expect(screen.queryByText(BANNER)).toBeNull())
})

test("a Schedule whose project is not found says so and shows no next run", async () => {
  renderPage([sched({ project_found: false })])
  expect(await screen.findByText("project not found")).toBeTruthy()
  expect(screen.queryByText(/^Next /)).toBeNull()
  expect(screen.getByText("Morning")).toBeTruthy()
})

test("clicking a row opens the Schedule", async () => {
  const router = renderPage([sched({})])
  fireEvent.click(await screen.findByText("Morning"))
  await waitFor(() => expect(router.state.location.pathname).toBe("/schedules/gx:morning"))
})

test("Enter on a row opens the Schedule", async () => {
  const router = renderPage([sched({})])
  const row = await screen.findByRole("listitem", { name: "Morning" })
  fireEvent.keyDown(row, { key: "Enter" })
  await waitFor(() => expect(router.state.location.pathname).toBe("/schedules/gx:morning"))
})
