import { cleanup, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, Outlet, RouterProvider, type RouteObject } from "react-router"
import { afterEach, beforeAll, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "./connection"
import type { ScheduleShowResult, SessionInfo } from "./generated/server"
import { HostedFound } from "./generated/ops"
import { appRoutes, pageRoutePatterns, type RouteFetchers } from "./routes"
import { ShellContext } from "./shell/ShellContext"
import { renderThemed } from "./ui/render"
import { DeviceContext, WideContext } from "./useWide"

vi.mock("./webPerf", () => ({ queueSample: vi.fn(), enableQueue: vi.fn(), dropQueue: vi.fn() }))

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
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

const emptyEpicList = async () => ({ epics: [], follow_ups: [] })
const notFound = async () => ({ kind: "not-found" as const })
const authOf = (totp_on: boolean) => async () =>
  ({ state: "signed-in", totp_on, setup_code_required: false }) as never
const noLogins = async () => []
const range = { first_sha: "a", second_sha: "b", merge_base: "a" }

interface Row {
  name: string
  /** The `pageRoutePatterns` entry this row covers. */
  pattern: string
  path: string | { pathname: string; state: unknown }
  /** Snapshot sessions; undefined means no snapshot yet. */
  sessions?: SessionInfo[]
  fetchers?: RouteFetchers
  /** The text only this state shows. */
  marker?: string
  /** The marker is an aria-label, not text. */
  byLabel?: boolean
  /** A marker that is neither text nor a label. */
  ready?: () => unknown
  /** Replaces `fetch` for pages that read through the default client. */
  fetch?: () => Promise<Response>
  /** The page draws no header: a full-window dialog covers the shell by design. */
  rest?: boolean
}

const loadingSpinner = { ready: () => document.querySelector(".chakra-spinner") }
const settingsLoading = { marker: "Loading", byLabel: true }

const rows: Row[] = [
  { name: "agents loading", pattern: "/agents/*", path: "/agents", marker: "Loading agents" },
  {
    name: "agents creating",
    pattern: "/agents/*",
    path: { pathname: "/agents/new1", state: { creating: true } },
    sessions: [],
    marker: "Starting chat…",
  },
  {
    name: "agents gone",
    pattern: "/agents/*",
    path: "/agents/x",
    sessions: [],
    marker: "Agent not found",
  },
  {
    name: "agents empty",
    pattern: "/agents/*",
    path: "/agents",
    sessions: [],
    marker: "No chats yet",
  },
  {
    name: "agents no selection",
    pattern: "/agents/*",
    path: "/agents",
    sessions: [sessionOf("a")],
    marker: "Pick a chat",
  },
  {
    name: "design failed",
    pattern: "/d/*",
    path: "/d/gx:map",
    fetchers: { epicList: failing },
    marker: "Could not load Designs",
  },
  {
    name: "design loading",
    pattern: "/d/*",
    path: "/d/gx:map",
    fetchers: { epicList: never },
    marker: "Loading Designs",
  },
  {
    name: "design with no address redirects to epics",
    pattern: "/d/*",
    path: "/d",
    fetchers: { epicList: never },
    marker: "Loading epics",
  },
  {
    name: "triage none",
    pattern: "/triage/*",
    path: "/triage/gx",
    fetchers: { triage: notFound },
    marker: "No Triage for gx",
  },
  {
    name: "triage loading",
    pattern: "/triage/*",
    path: "/triage/gx",
    fetchers: { triage: never },
    marker: "Loading Triage",
  },
  {
    name: "triage with no project redirects to epics",
    pattern: "/triage/*",
    path: "/triage",
    fetchers: { epicList: failing },
    marker: "Could not load epics",
  },
  {
    name: "schedule fresh failed",
    pattern: "/schedules/new",
    path: "/schedules/new",
    fetchers: { schedules: failing },
    marker: "Could not load the projects",
  },
  {
    name: "schedule fresh loading",
    pattern: "/schedules/new",
    path: "/schedules/new",
    fetchers: { schedules: never },
    marker: "Loading Schedule",
  },
  {
    name: "schedule existing failed",
    pattern: "/schedules/*",
    path: "/schedules/gx:morning",
    fetchers: { scheduleShow: failing },
    marker: "Could not load the Schedule",
  },
  {
    name: "schedule existing refused",
    pattern: "/schedules/*",
    path: "/schedules/gx:gone",
    fetchers: { scheduleShow: async () => refusedShow },
    marker: "no schedule gx:gone",
  },
  {
    name: "schedule existing loading",
    pattern: "/schedules/*",
    path: "/schedules/gx:morning",
    fetchers: { scheduleShow: never },
    marker: "Loading Schedule",
  },
  {
    name: "schedule editor new",
    pattern: "/schedules/new",
    path: "/schedules/new",
    fetchers: { schedules: async () => scheduleList },
    marker: "← Scheduled prompts",
  },
  {
    name: "schedule editor existing",
    pattern: "/schedules/*",
    path: "/schedules/gx:morning",
    fetchers: { scheduleShow: async () => shownSchedule },
    marker: "← Scheduled prompts",
  },
  {
    name: "file failed",
    pattern: "/files/*",
    path: "/files/tmp/a.md",
    fetchers: { fileResolve: failing },
    marker: "Could not load file",
  },
  {
    name: "file loading",
    pattern: "/files/*",
    path: "/files/tmp/a.md",
    fetchers: { fileResolve: never },
    marker: "Loading file",
    byLabel: true,
  },
  {
    name: "file found bar",
    pattern: "/files/*",
    path: "/files/tmp/a.md",
    fetchers: {
      fileResolve: async (p) => ({ candidate: p, verdict: HostedFound, path: p, size: 3 }),
      fileText: never,
    },
    marker: "/tmp/a.md",
  },
  {
    name: "file not-found bar",
    pattern: "/files/*",
    path: "/files/tmp/a.md",
    fetchers: { fileResolve: async (p) => ({ candidate: p, verdict: "missing" }) },
    marker: "/tmp/a.md",
  },

  {
    name: "epics loading",
    pattern: "/epics",
    path: "/epics",
    fetchers: { epicList: never },
    marker: "Loading epics",
  },
  {
    name: "epics failed",
    pattern: "/epics",
    path: "/epics",
    fetchers: { epicList: failing },
    marker: "Could not load epics",
  },
  {
    name: "epics empty",
    pattern: "/epics",
    path: "/epics",
    fetchers: { epicList: emptyEpicList },
    marker: "No epics yet.",
  },
  {
    name: "archive loading",
    pattern: "/archive",
    path: "/archive",
    fetchers: { archived: never },
    marker: "Loading archive",
  },
  {
    name: "archive failed",
    pattern: "/archive",
    path: "/archive",
    fetchers: { archived: failing },
    marker: "Could not load the archive",
  },
  {
    name: "archive empty",
    pattern: "/archive",
    path: "/archive",
    fetchers: { archived: async () => ({ epics: [] }) },
    marker: "Nothing is archived",
  },
  {
    name: "backlog loading",
    pattern: "/backlog",
    path: "/backlog",
    fetchers: { backlog: never },
    marker: "Loading backlog",
  },
  {
    name: "backlog failed",
    pattern: "/backlog",
    path: "/backlog",
    fetchers: { backlog: failing },
    marker: "Could not load the backlog",
  },
  {
    name: "backlog empty",
    pattern: "/backlog",
    path: "/backlog",
    fetchers: { backlog: async () => ({ projects: [] }) },
    marker: "No epics yet.",
  },
  {
    name: "initiatives loading",
    pattern: "/initiatives",
    path: "/initiatives",
    fetchers: { initiatives: never },
    marker: "Loading initiatives",
  },
  {
    name: "initiatives failed",
    pattern: "/initiatives",
    path: "/initiatives",
    fetchers: { initiatives: failing },
    marker: "Could not load the initiatives",
  },
  {
    name: "initiatives empty",
    pattern: "/initiatives",
    path: "/initiatives",
    fetchers: { initiatives: async () => ({ projects: [] }) },
    marker: "gx initiatives new",
  },
  {
    name: "initiative loading",
    pattern: "/initiatives/*",
    path: "/initiatives/gx:payments-rewrite",
    fetchers: { initiative: never },
    marker: "Loading initiative",
  },
  {
    name: "initiative failed",
    pattern: "/initiatives/*",
    path: "/initiatives/gx:payments-rewrite",
    fetchers: { initiative: failing },
    marker: "Could not load the initiative",
  },
  {
    name: "initiative not found",
    pattern: "/initiatives/*",
    path: "/initiatives/gx:payments-rewrite",
    fetchers: { initiative: notFound },
    marker: "Initiative not found",
  },
  {
    name: "cleanup loading",
    pattern: "/cleanup",
    path: "/cleanup?project=p",
    fetchers: { cleanup: never, epicList: never },
    marker: "Loading cleanup",
  },
  {
    name: "cleanup failed",
    pattern: "/cleanup",
    path: "/cleanup?project=p",
    fetchers: { cleanup: failing, epicList: never },
    marker: "Could not load cleanup",
  },
  {
    name: "cleanup empty",
    pattern: "/cleanup",
    path: "/cleanup?project=p",
    fetchers: {
      cleanup: async () => ({ project: "p", epics: [], worktrees: [], attic_refs: [] }) as never,
      epicList: never,
    },
    marker: "Nothing to clean up",
  },
  {
    name: "stats loading",
    pattern: "/stats",
    path: "/stats",
    fetchers: { stats: never, epicList: never },
    marker: "Loading stats",
  },
  {
    name: "stats failed",
    pattern: "/stats",
    path: "/stats",
    fetchers: { stats: failing, epicList: never },
    marker: "Could not load stats",
  },
  {
    name: "tips loading",
    pattern: "/tips",
    path: "/tips",
    fetchers: { tips: never },
    marker: "Loading tips",
  },
  {
    name: "tips failed",
    pattern: "/tips",
    path: "/tips",
    fetchers: { tips: failing },
    marker: "Could not load tips",
  },
  {
    name: "tips empty",
    pattern: "/tips",
    path: "/tips",
    fetchers: { tips: async () => [] },
    marker: "No tips for this machine",
  },
  {
    name: "schedules loading",
    pattern: "/schedules",
    path: "/schedules",
    fetchers: { schedules: never },
    marker: "Loading Scheduled prompts",
  },
  {
    name: "schedules failed",
    pattern: "/schedules",
    path: "/schedules",
    fetchers: { schedules: failing },
    marker: "Could not load the Scheduled prompts",
  },
  {
    name: "schedules empty",
    pattern: "/schedules",
    path: "/schedules",
    fetchers: { schedules: async () => scheduleList },
    marker: "No scheduled prompts yet.",
  },
  {
    name: "diff loading",
    pattern: "/diff",
    path: "/diff?project=p&base=a&tip=b",
    fetchers: { diffFiles: never },
    marker: "Loading diff",
  },
  {
    name: "diff failed",
    pattern: "/diff",
    path: "/diff?project=p&base=a&tip=b",
    fetchers: { diffFiles: failing },
    marker: "Could not load the diff",
  },
  {
    name: "diff unknown ref",
    pattern: "/diff",
    path: "/diff?project=p&base=a&tip=b",
    fetchers: { diffFiles: async () => ({ kind: "unknown-ref", side: "first" }) as never },
    marker: "Unknown ref",
  },
  {
    name: "diff empty",
    pattern: "/diff",
    path: "/diff?project=p&base=a&tip=b",
    fetchers: {
      diffFiles: async () => ({ kind: "files", files: { ...range, files: [] } }) as never,
    },
    marker: "No changes",
  },

  {
    name: "epic loading",
    pattern: "/e/*",
    path: "/e/gx:map",
    fetchers: { epic: never, epicList: never },
    marker: "Loading epic",
  },
  {
    name: "epic not found",
    pattern: "/e/*",
    path: "/e/gx:map",
    fetchers: { epic: notFound, epicList: never },
    marker: "Epic not found",
  },
  {
    name: "archived epic loading",
    pattern: "/archive/*",
    path: "/archive/gx:map",
    fetchers: { archivedEpic: never, epicList: never },
    marker: "Loading epic",
  },
  {
    name: "archived epic not found",
    pattern: "/archive/*",
    path: "/archive/gx:map",
    fetchers: { archivedEpic: notFound, epicList: never },
    marker: "Epic not found",
  },
  {
    name: "ticket loading",
    pattern: "/t/*",
    path: "/t/gx:map/01",
    fetchers: { ticket: never },
    marker: "Loading ticket",
  },
  {
    name: "ticket not found",
    pattern: "/t/*",
    path: "/t/gx:map/01",
    fetchers: { ticket: notFound },
    marker: "Ticket not found",
  },
  {
    name: "answer loading",
    pattern: "/answer/*",
    path: "/answer/gx:map/01",
    fetchers: { ticket: never },
    marker: "Loading ticket",
  },
  {
    name: "answer not found",
    pattern: "/answer/*",
    path: "/answer/gx:map/01",
    fetchers: { ticket: notFound },
    marker: "Ticket not found",
  },

  {
    name: "draft loading",
    pattern: "/drafts/:id",
    path: "/drafts/d1",
    fetch: never,
    ...loadingSpinner,
  },
  {
    name: "draft not found",
    pattern: "/drafts/:id",
    path: "/drafts/d1",
    fetch: failing,
    marker: "Draft not found",
  },
  {
    name: "chat draft loading",
    pattern: "/chat-drafts/:id",
    path: "/chat-drafts/d1",
    fetch: never,
    ...loadingSpinner,
  },
  {
    name: "chat draft not found",
    pattern: "/chat-drafts/:id",
    path: "/chat-drafts/d1",
    fetch: failing,
    marker: "Draft not found",
  },

  {
    name: "settings home loading",
    pattern: "/settings",
    path: "/settings",
    fetchers: { authState: never, settings: never, logins: never },
    ...settingsLoading,
  },
  {
    name: "settings section loading",
    pattern: "/settings/s/:section",
    path: "/settings/s/agents",
    fetchers: { authState: never, settings: never, logins: never },
    ...settingsLoading,
  },
  {
    name: "settings agents redirect",
    pattern: "/settings/agents",
    path: "/settings/agents",
    fetchers: { authState: never, settings: never, logins: never },
    ...settingsLoading,
  },
  {
    name: "settings project loading",
    pattern: "/settings/p/:project",
    path: "/settings/p/gx",
    fetchers: { authState: never, settings: never, epicList: never },
    ...settingsLoading,
  },
  {
    name: "settings project failed",
    pattern: "/settings/p/:project",
    path: "/settings/p/gx",
    fetchers: { authState: never, settings: failing, epicList: never },
    marker: "Project not found",
  },
  {
    name: "settings project section failed",
    pattern: "/settings/p/:project/s/:section",
    path: "/settings/p/gx/s/agents",
    fetchers: { authState: never, settings: failing, epicList: never },
    marker: "Project not found",
  },
  {
    name: "web logins loading",
    pattern: "/settings/logins",
    path: "/settings/logins",
    fetchers: { logins: never },
    ...settingsLoading,
  },
  {
    name: "web login loading",
    pattern: "/settings/logins/:id",
    path: "/settings/logins/x",
    fetchers: { logins: never },
    ...settingsLoading,
  },
  {
    name: "web login gone",
    pattern: "/settings/logins/:id",
    path: "/settings/logins/x",
    fetchers: { logins: noLogins },
    marker: "This web login no longer exists.",
  },
  {
    name: "password loading",
    pattern: "/settings/password",
    path: "/settings/password",
    fetchers: { authState: never, logins: never },
    ...settingsLoading,
  },
  {
    name: "authenticator loading",
    pattern: "/settings/authenticator",
    path: "/settings/authenticator",
    fetchers: { authState: never, logins: never },
    ...settingsLoading,
  },
  {
    name: "authenticator off",
    pattern: "/settings/authenticator",
    path: "/settings/authenticator",
    fetchers: { authState: authOf(false), logins: noLogins },
    marker: "The authenticator is turned off.",
  },
  {
    name: "certificate loading",
    pattern: "/settings/certificate",
    path: "/settings/certificate",
    fetchers: { authState: never },
    ...settingsLoading,
  },
  {
    name: "unknown settings page",
    pattern: "/settings/*",
    path: "/settings/zzz",
    marker: "Settings",
  },

  // A full-window modal on made-up state: it makes the page inert and covers the burger by design.
  { name: "rest break", pattern: "/rest/break", path: "/rest/break", rest: true },
  { name: "rest bedtime", pattern: "/rest/bedtime", path: "/rest/bedtime", rest: true },
]

test("every page route pattern has a row", () => {
  const covered = new Set(rows.map((r) => r.pattern))
  expect(pageRoutePatterns().filter((p) => !covered.has(p))).toEqual([])
})

test.each(rows)("$name claims a shell header and shows one burger", async (row) => {
  if (row.fetch) vi.stubGlobal("fetch", vi.fn(row.fetch))
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

  if (row.rest) {
    expect(await screen.findByRole("dialog", { name: "Rest" })).toBeTruthy()
    expect(claims).toBe(0)
    return
  }
  if (row.ready) {
    await waitFor(() => expect(row.ready!()).toBeTruthy())
  } else if (row.byLabel) {
    await screen.findAllByLabelText(row.marker!)
  } else {
    await screen.findAllByText(row.marker!, { exact: false })
  }
  expect(claims).toBe(1)
  expect(screen.getAllByRole("button", { name: "Show sidebar" })).toHaveLength(1)
})
