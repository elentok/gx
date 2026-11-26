import { act, cleanup, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, beforeAll, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { FocusReport, SessionInfo } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { PostFocus } from "./focus"

// The route lazy-loads AgentsPage; under load the first import can outlast a waitFor timeout.
beforeAll(() => import("../agents/AgentsPage"))
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

const session = (id: string, changed: string): SessionInfo => ({
  id,
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" } as SessionInfo["status"],
  status_changed_at: changed,
  run_started_at: changed,
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
})

function setVisibility(v: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { value: v, configurable: true })
  document.dispatchEvent(new Event("visibilitychange"))
}

// The report of a page that shows a session's working card.
const card = (focused: boolean, id: string) => ({
  client: "tab-1",
  focused,
  session: id,
  card: true,
  seq: expect.any(Number),
})

function render(focus: PostFocus) {
  let state = {
    snapshot: { sessions: [session("s-a", "2026-01-01T00:00:00Z")] },
    connected: true,
    loadedOnce: true,
    tick: 0,
    streams: 1,
  } as unknown as ConnectionState
  const listeners = new Set<() => void>()
  const conn = {
    clientId: "tab-1",
    getState: () => state,
    subscribe: (l: () => void) => {
      listeners.add(l)
      return () => {
        listeners.delete(l)
      }
    },
    onEvent: () => () => {},
  } as unknown as Connection
  const update = (patch: Partial<ConnectionState>) => {
    state = { ...state, ...patch }
    for (const l of listeners) l()
  }
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({
        epics: [
          {
            address: "gx:a",
            project: "gx",
            name: "a",
            kind: "map",
            state: "designing",
            done: 0,
            total: 0,
            queued: false,
            queueable: false,
          },
        ],
        follow_ups: [],
      }),
      epic: async () => ({
        kind: "ok",
        detail: {
          address: "gx:a",
          kind: "map",
          summary: "",
          done: 0,
          total: 0,
          agent_time: 0,
          cost_total: 0,
          stats: {},
          rows: [],
          queueable: false,
          queued: false,
          steps: [],
        },
      }),
      chat: async () => ({
        kind: "ok",
        detail: { conversation_id: "c1", messages: [], questions: [] },
      }),
      focus,
    }),
    { initialEntries: ["/d/gx:a"] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return { update, router }
}

test("reports on open, on visibility change and on a session switch", async () => {
  setVisibility("visible")
  const focus = vi.fn<PostFocus>(async () => ({}))
  const { update } = render(focus)
  await waitFor(() => expect(focus).toHaveBeenCalledWith(card(true, "s-a")))

  act(() => setVisibility("hidden"))
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(false, "s-a")))

  act(() =>
    update({
      snapshot: {
        sessions: [session("s-a", "2026-01-01T00:00:00Z"), session("s-b", "2026-02-01T00:00:00Z")],
      } as ConnectionState["snapshot"],
    }),
  )
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(false, "s-b")))
  setVisibility("visible")
})

test("a new stream in a hidden tab reports not focused, and showing it reports focused", async () => {
  setVisibility("visible")
  const focus = vi.fn<PostFocus>(async () => ({}))
  const { update } = render(focus)
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(true, "s-a")))

  act(() => setVisibility("hidden"))
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(false, "s-a")))
  const before = focus.mock.calls.length
  act(() => update({ streams: 2 }))
  await waitFor(() => expect(focus.mock.calls.length).toBe(before + 1))
  expect(focus).toHaveBeenLastCalledWith(card(false, "s-a"))

  act(() => setVisibility("visible"))
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(true, "s-a")))
})

test("a step page and the chat page report the card, with numbers growing across the switch", async () => {
  setVisibility("visible")
  const reports: FocusReport[] = []
  // The chat page posts through fetch, the step page through the injected post.
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: { body: string }) => {
      if (url === "/v1/sessions/focus") reports.push(JSON.parse(init!.body))
      return new Response("{}")
    }),
  )
  const { router } = render(async (r) => reports.push(r))
  await waitFor(() => expect(reports.at(-1)).toEqual(card(true, "s-a")))
  const onStepPage = reports.length

  await act(() => router.navigate("/agents/s-a"))
  await waitFor(() => expect(reports).toHaveLength(onStepPage + 2))
  const [left, next] = reports.slice(onStepPage)
  expect(left).toEqual({ client: "tab-1", focused: false, session: "", seq: expect.any(Number) })
  expect(left).not.toHaveProperty("card")
  expect(next).toEqual(card(true, "s-a"))
  const seqs = reports.map((r) => r.seq!)
  expect(seqs).toEqual([...new Set(seqs)].sort((a, b) => a - b))
})

test("leaving the Design page reports no focus and no session", async () => {
  setVisibility("visible")
  const focus = vi.fn<PostFocus>(async () => ({}))
  const { update, router } = render(focus)
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(true, "s-a")))

  act(() =>
    update({
      snapshot: {
        sessions: [session("s-a", "2026-01-01T00:00:00Z"), session("s-b", "2026-02-01T00:00:00Z")],
      } as ConnectionState["snapshot"],
    }),
  )
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(card(true, "s-b")))
  const left = { client: "tab-1", focused: false, session: "", seq: expect.any(Number) }
  expect(focus).not.toHaveBeenCalledWith(left)

  await act(() => router.navigate("/"))
  await waitFor(() => expect(focus).toHaveBeenLastCalledWith(left))
})
