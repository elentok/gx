import { cleanup, screen } from "@testing-library/react"
import { createMemoryRouter, Outlet, RouterProvider, type RouteObject } from "react-router"
import { afterEach, beforeAll, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "./connection"
import type { ScheduleShowResult, SessionInfo } from "./generated/server"
import { HostedFound } from "./generated/ops"
import { appRoutes, type RouteFetchers } from "./routes"
import { ShellContext } from "./shell/ShellContext"
import { renderThemed } from "./ui/render"
import { DeviceContext, WideContext } from "./useWide"

vi.mock("./webPerf", () => ({ queueSample: vi.fn(), enableQueue: vi.fn(), dropQueue: vi.fn() }))

afterEach(cleanup)
// The routes lazy-load their pages; load them before any row waits on a marker.
beforeAll(() => import("./agents/AgentsPage"))

const never = <T,>() => new Promise<T>(() => {})
const failing = () => Promise.reject(new Error("down"))

const sessionOf = (id: string): SessionInfo => ({
  id,
  project: "gx",
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
})

const shownSchedule: ScheduleShowResult = {
  refused: false,
  reason: "",
  message: "",
  prompt: "Summarise the night",
  last_runs: [],
  schedule: {
    name: "Morning",
    slug: "morning",
    project: "gx",
    days: ["mon"],
    times: ["08:30"],
    harness: "",
    tier: "",
    default_harness: "claude",
    late_limit_hours: 2,
    switch: true,
    project_found: true,
    next_run: "",
    updated_at: "",
  },
}
const refusedShow: ScheduleShowResult = {
  ...shownSchedule,
  refused: true,
  reason: "unknown-schedule",
  message: "no schedule gx:gone",
}
const scheduleList = {
  refused: false,
  reason: "",
  message: "",
  schedules: [],
  projects: ["gx"],
}

interface Row {
  name: string
  path: string | { pathname: string; state: unknown }
  /** Snapshot sessions; undefined means no snapshot yet. */
  sessions?: SessionInfo[]
  fetchers?: RouteFetchers
  /** The text only this state shows. */
  marker: string
  /** The marker is an aria-label, not text. */
  byLabel?: boolean
}

const rows: Row[] = [
  { name: "agents loading", path: "/agents", marker: "Loading agents" },
  {
    name: "agents creating",
    path: { pathname: "/agents/new1", state: { creating: true } },
    sessions: [],
    marker: "Creating chat…",
  },
  { name: "agents gone", path: "/agents/x", sessions: [], marker: "Session not found" },
  { name: "agents empty", path: "/agents", sessions: [], marker: "No sessions yet" },
  {
    name: "agents no selection",
    path: "/agents",
    sessions: [sessionOf("a")],
    marker: "Pick a session",
  },
  {
    name: "design failed",
    path: "/d/gx:map",
    fetchers: { epicList: failing },
    marker: "Could not load Designs",
  },
  {
    name: "design loading",
    path: "/d/gx:map",
    fetchers: { epicList: never },
    marker: "Loading Designs",
  },
  {
    name: "triage none",
    path: "/triage/gx",
    fetchers: { triage: async () => ({ kind: "not-found" }) },
    marker: "No Triage for gx",
  },
  {
    name: "triage loading",
    path: "/triage/gx",
    fetchers: { triage: never },
    marker: "Loading Triage",
  },
  {
    name: "schedule fresh failed",
    path: "/schedules/new",
    fetchers: { schedules: failing },
    marker: "Could not load the projects",
  },
  {
    name: "schedule fresh loading",
    path: "/schedules/new",
    fetchers: { schedules: never },
    marker: "Loading Schedule",
  },
  {
    name: "schedule existing failed",
    path: "/schedules/gx:morning",
    fetchers: { scheduleShow: failing },
    marker: "Could not load the Schedule",
  },
  {
    name: "schedule existing refused",
    path: "/schedules/gx:gone",
    fetchers: { scheduleShow: async () => refusedShow },
    marker: "no schedule gx:gone",
  },
  {
    name: "schedule existing loading",
    path: "/schedules/gx:morning",
    fetchers: { scheduleShow: never },
    marker: "Loading Schedule",
  },
  {
    name: "schedule editor new",
    path: "/schedules/new",
    fetchers: { schedules: async () => scheduleList },
    marker: "← Scheduled prompts",
  },
  {
    name: "schedule editor existing",
    path: "/schedules/gx:morning",
    fetchers: { scheduleShow: async () => shownSchedule },
    marker: "← Scheduled prompts",
  },
  {
    name: "file failed",
    path: "/files/tmp/a.md",
    fetchers: { fileResolve: failing },
    marker: "Could not load file",
  },
  {
    name: "file loading",
    path: "/files/tmp/a.md",
    fetchers: { fileResolve: never },
    marker: "Loading file",
    byLabel: true,
  },
  {
    name: "file found bar",
    path: "/files/tmp/a.md",
    fetchers: {
      fileResolve: async (p) => ({ candidate: p, verdict: HostedFound, path: p, size: 3 }),
      fileText: never,
    },
    marker: "/tmp/a.md",
  },
  {
    name: "file not-found bar",
    path: "/files/tmp/a.md",
    fetchers: { fileResolve: async (p) => ({ candidate: p, verdict: "missing" }) },
    marker: "/tmp/a.md",
  },
]

test.each(rows)("$name claims a shell header and shows one burger", async (row) => {
  const state = {
    snapshot: row.sessions && { sessions: row.sessions },
    connected: true,
    loadedOnce: !!row.sessions,
    liveSnapshot: !!row.sessions,
    streams: 1,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    clientId: "c1",
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection

  let claims = 0
  const shell = {
    sidebarOpen: false,
    toggle: () => {},
    close: () => {},
    closeSliding: () => {},
    claimHeader: () => {
      claims++
      return () => {
        claims--
      }
    },
  }
  const pages = appRoutes(row.fetchers)[0].children as RouteObject[]
  const router = createMemoryRouter(
    [
      {
        element: (
          <ShellContext.Provider value={shell}>
            <Outlet />
          </ShellContext.Provider>
        ),
        children: pages,
      },
    ],
    { initialEntries: [row.path] },
  )
  renderThemed(
    <DeviceContext.Provider value="phone">
      <WideContext.Provider value={false}>
        <ConnectionContext.Provider value={conn}>
          <RouterProvider router={router} />
        </ConnectionContext.Provider>
      </WideContext.Provider>
    </DeviceContext.Provider>,
  )

  await (row.byLabel
    ? screen.findAllByLabelText(row.marker)
    : screen.findAllByText(row.marker, { exact: false }))
  expect(claims).toBe(1)
  expect(screen.getAllByRole("button", { name: "Show sidebar" })).toHaveLength(1)
})
