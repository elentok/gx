import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import canned from "../canned-detail.json"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { TicketDetail } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { FetchDetail } from "./detail"

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

const base = { ...canned, epic: "gx:demo", map_epic: false, related: [], runs: [] } as TicketDetail
const plain: TicketDetail = {
  ...base,
  status: "open",
  pause_reason: undefined,
  rounds: [],
  latest: undefined,
  chat: [],
  state: "",
  actions: [],
}

// The state object must be stable: useSyncExternalStore loops on a fresh one per call.
const state = {
  connected: true,
  loadedOnce: true,
  tick: 0,
  snapshot: { tickets: [], epics: [] },
} as unknown as ConnectionState
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection

function renderTicket(current: () => TicketDetail) {
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail: current() })
  const router = createMemoryRouter(
    appRoutes({ ticket: fetchDetail, epicList: async () => ({ epics: [], follow_ups: [] }) }),
    { initialEntries: ["/t/gx:demo/12"] },
  )
  return renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
}

const card = () => screen.queryByRole("group", { name: "Action card" })

test("an open question shows the card first under the header, with its chat and rounds", async () => {
  renderTicket(() => base)
  const c = await screen.findByRole("group", { name: "Action card" })
  const header = screen.getByRole("heading", { level: 1 })
  expect(header.compareDocumentPosition(c) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(c.textContent).toContain("Should the cache live in Redis or in process memory?")
  expect(c.textContent).toContain("Earlier rounds (2)")
  expect(c.querySelector('[role="form"]')).toBeTruthy()
})

test("a permission prompt on a running ticket shows the card", async () => {
  const detail: TicketDetail = {
    ...plain,
    status: "claimed",
    permission: { request_id: "r1", tool: "Bash", description: "run ls" },
  }
  renderTicket(() => detail)
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("Needs approval")
  expect(screen.getByRole("button", { name: "Allow once" })).toBeTruthy()
})

test("a Paused ticket shows its pause text in the card next to its action", async () => {
  const detail: TicketDetail = {
    ...plain,
    status: "needs-answer",
    pause_reason: "zero-commit",
    pause_text: "No commits",
    state: "tok-2",
    actions: ["retry"],
    action_info: { retry: { label: "Retry", tooltip: "Start a fresh agent now." } },
  }
  renderTicket(() => detail)
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("No commits")
  expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
})

const info = {
  retry: { label: "Retry", tooltip: "Start a fresh agent now." },
  "start-over": { label: "Start over", tooltip: "Throw away the work and run again." },
  merge: { label: "Merge again", tooltip: "Merge the commits again." },
  resume: { label: "Resume", tooltip: "Lift the pause." },
  "resume-on-worker": {
    label: "Resume on worker",
    tooltip: "Run the ticket again on its tier's model.",
  },
  "reopen-and-retry": { label: "Reopen and retry", tooltip: "Reopen the ticket." },
}
const repair = (
  actions: (keyof typeof info)[],
  extra: Partial<TicketDetail> = {},
): TicketDetail => ({
  ...plain,
  status: "needs-repair",
  pause_reason: "iteration-error",
  pause_text: "Agent crashed",
  state: "tok-3",
  actions,
  action_info: info,
  ...extra,
})
// The shell's limits tray and follow-ups button read on their own; keep that out of the mock the tests assert on.
function stubFetch(mock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal("fetch", (url: string, init?: RequestInit) =>
    url.startsWith("/v1/usage/") || url === "/v1/epics"
      ? Promise.resolve(new Response("", { status: 500 }))
      : mock(url, init),
  )
}
const posted = (fetchMock: ReturnType<typeof vi.fn>) =>
  fetchMock.mock.calls.map(([url, init]) => [url, JSON.parse((init as RequestInit).body as string)])

test("a repair pause shows one button per action with its label and tooltip", async () => {
  renderTicket(() => repair(["retry", "start-over"]))
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("Agent crashed")
  expect(screen.getByRole("button", { name: "Retry" }).getAttribute("title")).toBe(
    "Start a fresh agent now.",
  )
  expect(screen.getByRole("button", { name: "Start over" }).getAttribute("title")).toBe(
    "Throw away the work and run again.",
  )
  expect(screen.queryByRole("button", { name: "Merge again" })).toBeNull()
})

test("stranded-commits reads Reopen and retry", async () => {
  renderTicket(() => repair(["reopen-and-retry"], { pause_reason: "stranded-commits" }))
  expect(await screen.findByRole("button", { name: "Reopen and retry" })).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Merge commits" })).toBeNull()
})

test("Retry posts the state token and shows a success toast", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({})))
  stubFetch(fetchMock)
  renderTicket(() => repair(["retry", "start-over"]))
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }))
  expect(await screen.findByText("Retry started")).toBeTruthy()
  expect(posted(fetchMock)).toEqual([
    ["/v1/tickets/retry", { address: "gx:demo/12", state: "tok-3" }],
  ])
})

test("Retry is disabled while the call runs", async () => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => (release = resolve))
  stubFetch(
    vi.fn(async () => {
      await gate
      return new Response(JSON.stringify({}))
    }),
  )
  renderTicket(() => repair(["retry", "start-over"]))
  const retry = (await screen.findByRole("button", { name: "Retry" })) as HTMLButtonElement
  fireEvent.click(retry)
  await waitFor(() => expect(retry.disabled).toBe(true))
  release()
  expect(await screen.findByText("Retry started")).toBeTruthy()
})

test("Resume posts to the resume route and shows a success toast", async () => {
  const fetchMock = vi.fn(
    async () => new Response(JSON.stringify({ data: { ticket: "12", status: "open" } })),
  )
  stubFetch(fetchMock)
  renderTicket(() => repair(["resume", "start-over"], { pause_reason: "manual" }))
  fireEvent.click(await screen.findByRole("button", { name: "Resume" }))
  expect(await screen.findByText("Resumed")).toBeTruthy()
  expect(posted(fetchMock)[0]).toEqual([
    "/v1/tickets/resume",
    { address: "gx:demo/12", state: "tok-3" },
  ])
})

test("a model-mismatch pause shows both models and one Resume on worker button that posts tier=worker", async () => {
  const fetchMock = vi.fn(
    async () => new Response(JSON.stringify({ data: { ticket: "12", status: "open" } })),
  )
  stubFetch(fetchMock)
  renderTicket(() =>
    repair(["resume-on-worker"], {
      pause_reason: "model-mismatch",
      model_ran: "claude-opus-4",
      model_expected: "claude-sonnet-4",
    }),
  )
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("Ran on Claude Opus 4, expected Claude Sonnet 4.")
  expect(screen.queryByRole("button", { name: "Resume" })).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "Resume on worker" }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  expect(posted(fetchMock)[0]).toEqual([
    "/v1/tickets/resume",
    { address: "gx:demo/12", state: "tok-3", tier: "worker" },
  ])
})

test("other pauses show no Resume on worker button", async () => {
  renderTicket(() => repair(["resume", "start-over"], { pause_reason: "manual" }))
  await screen.findByRole("button", { name: "Resume" })
  expect(screen.queryByRole("button", { name: "Resume on worker" })).toBeNull()
})

test("Merge again posts to the merge route", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ data: {} })))
  stubFetch(fetchMock)
  renderTicket(() => repair(["merge", "start-over"], { pause_reason: "merge-conflict" }))
  fireEvent.click(await screen.findByRole("button", { name: "Merge again" }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  expect(posted(fetchMock)[0]).toEqual([
    "/v1/tickets/merge",
    { address: "gx:demo/12", state: "tok-3" },
  ])
})

test("Start over asks for a reason first and never sends branch deletion", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ data: {} })))
  stubFetch(fetchMock)
  renderTicket(() => repair(["retry", "start-over"]))
  fireEvent.click(await screen.findByRole("button", { name: "Start over" }))
  const dialog = screen.getByRole("dialog", { name: "Start over" })
  expect(fetchMock).not.toHaveBeenCalled()
  expect(within(dialog).getByText(/discards the branch's commits.*backup branch/i)).toBeTruthy()
  const reason = within(dialog).getByLabelText("Reason")
  expect(reason.getAttribute("autocomplete")).toBe("off")
  expect(reason.getAttribute("name")).toBe("recovery-reason")
  fireEvent.change(reason, { target: { value: "wrong approach" } })
  fireEvent.click(within(dialog).getByRole("button", { name: "Start over" }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  const [url, body] = posted(fetchMock)[0]!
  expect(url).toBe("/v1/tickets/start-over")
  expect(body).toEqual({ address: "gx:demo/12", state: "tok-3", reason: "wrong approach" })
  expect(body).not.toHaveProperty("delete_branch")
})

test("Start over closes on Escape and drops the reason; Cancel closes; the field expands", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ data: {} })))
  stubFetch(fetchMock)
  renderTicket(() => repair(["retry", "start-over"]))
  fireEvent.click(await screen.findByRole("button", { name: "Start over" }))
  let dialog = screen.getByRole("dialog", { name: "Start over" })
  expect(
    within(dialog).getByRole("button", { name: "Open the note in a large editor" }),
  ).toBeTruthy()
  fireEvent.change(within(dialog).getByLabelText("Reason"), { target: { value: "typed" } })
  fireEvent.keyDown(within(dialog).getByLabelText("Reason"), { key: "Escape" })
  expect(screen.queryByRole("dialog")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "Start over" }))
  dialog = screen.getByRole("dialog", { name: "Start over" })
  expect((within(dialog).getByLabelText("Reason") as HTMLTextAreaElement).value).toBe("")
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }))
  expect(screen.queryByRole("dialog")).toBeNull()
  expect(fetchMock).not.toHaveBeenCalled()
})

test("Start over's Confirm is disabled until a reason is typed", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({ data: {} })))
  stubFetch(fetchMock)
  renderTicket(() => repair(["retry", "start-over"]))
  fireEvent.click(await screen.findByRole("button", { name: "Start over" }))
  const dialog = screen.getByRole("dialog", { name: "Start over" })
  const confirm = within(dialog).getByRole("button", { name: "Start over" }) as HTMLButtonElement
  expect(confirm.disabled).toBe(true)
  fireEvent.change(within(dialog).getByLabelText("Reason"), { target: { value: "   " } })
  expect(confirm.disabled).toBe(true)
  fireEvent.change(within(dialog).getByLabelText("Reason"), { target: { value: "wrong approach" } })
  expect(confirm.disabled).toBe(false)
})

test("a Retry refused because the agent is working offers Stop agent and retry, which sends stop", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          refused: true,
          reason: "agent-working",
          message: "agent is still working",
        }),
      ),
    )
    .mockResolvedValueOnce(new Response(JSON.stringify({})))
  stubFetch(fetchMock)
  renderTicket(() => repair(["retry", "start-over"]))
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }))
  fireEvent.click(await screen.findByRole("button", { name: "Stop agent and retry" }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  expect(posted(fetchMock)[1]).toEqual([
    "/v1/tickets/retry",
    { address: "gx:demo/12", state: "tok-3", stop: true },
  ])
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "Stop agent and retry" })).toBeNull(),
  )
})

test("a paused ticket whose run is still finishing says so and offers no Retry", async () => {
  renderTicket(() => repair([], { run_finishing: true }))
  expect(
    await screen.findByText("A run is still finishing. Recovery actions return when it settles."),
  ).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull()
})

test("a stale refusal refetches the detail", async () => {
  let fetched = 0
  const fetchMock = vi.fn(
    async () =>
      new Response(
        JSON.stringify({ refused: true, reason: "stale", message: "The pause changed." }),
      ),
  )
  stubFetch(fetchMock)
  renderTicket(() => {
    fetched++
    return repair(["retry", "start-over"])
  })
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }))
  const before = fetched
  await waitFor(() => expect(fetched).toBeGreaterThan(before))
})

test("an Answered ticket keeps the last question and the answer", async () => {
  const detail: TicketDetail = {
    ...base,
    status: "open",
    answered: true,
    latest: { ...base.latest!, outcome: "answered", answer: { source: "web", text: "Redis" } },
    chat: [...base.chat, { from: "web", text: "Redis" }],
    actions: [],
  }
  renderTicket(() => detail)
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("Should the cache live in Redis or in process memory?")
  expect(c.textContent).toContain("Redis")
})

test("a sent answer keeps its result on screen after the refetch", async () => {
  let detail = base
  stubFetch(
    vi.fn(async () => {
      detail = {
        ...base,
        status: "open",
        answered: true,
        latest: { ...base.latest!, outcome: "answered", answer: { source: "web", text: "Redis" } },
        actions: [],
      }
      return new Response(JSON.stringify({ outcome: "queued" }))
    }),
  )
  renderTicket(() => detail)
  fireEvent.change(await screen.findByLabelText(/^Answer to question/), {
    target: { value: "Redis" },
  })
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  const result = await screen.findByRole("status", { name: "Answer result" })
  expect(result.textContent).toContain("Queued ·")
  await waitFor(() => expect(screen.queryByLabelText(/^Answer to question/)).toBeNull())
  expect(screen.getByRole("status", { name: "Answer result" })).toBeTruthy()
  expect(card()).toBeTruthy()
})

test("a done ticket and a plain open ticket show no card and no earlier rounds", async () => {
  renderTicket(() => ({
    ...base,
    status: "done",
    rounds: base.rounds,
    latest: { ...base.latest!, outcome: "answered" },
    actions: [],
  }))
  await screen.findByRole("heading", { level: 1 })
  expect(card()).toBeNull()
  expect(screen.queryByText(/Earlier rounds/)).toBeNull()
  cleanup()
  renderTicket(() => plain)
  await screen.findByRole("heading", { level: 1 })
  expect(card()).toBeNull()
  expect(screen.queryByText(/Earlier rounds/)).toBeNull()
})

test("a recommendation shows its diagnosis and marks one button Recommended", async () => {
  renderTicket(() =>
    repair(["retry", "start-over"], {
      recommendation: { verb: "start-over", diagnosis: "The branch is unrecoverable." },
    }),
  )
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("The branch is unrecoverable.")
  expect(screen.getAllByText("Recommended")).toHaveLength(1)
  expect(
    within(screen.getByRole("button", { name: /Start over/ })).getByText("Recommended"),
  ).toBeTruthy()
})

test("a round the agent was prompted on shows the Prompt sent line", async () => {
  const prompt = "Prompt sent to the running agent at 14:05"
  renderTicket(() => ({
    ...base,
    latest: { ...base.latest!, prompt, prompted_at: "2026-10-26T14:05:00Z" },
  }))
  expect(await screen.findByText(prompt)).toBeTruthy()
})

test("a prompted Retry on a zero-commit card shows that the ticket stays paused", async () => {
  const body = {
    data: {
      ticket: "12",
      status: "prompted",
      message: "the agent is still running and was asked to continue",
    },
  }
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(body)))
  stubFetch(fetchMock)
  renderTicket(() => repair(["retry"], { pause_reason: "zero-commit" }))
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }))
  expect(
    await screen.findByText(
      "Sent to the running agent. The ticket stays paused until it finishes.",
    ),
  ).toBeTruthy()
  expect(fetchMock).toHaveBeenCalledWith("/v1/tickets/retry", expect.anything())
})

test("a gate card with a live task names the task and shows how it is doing", async () => {
  renderTicket(() =>
    repair(["retry"], {
      pause_reason: "background-task-gate",
      pause_text: "",
      task: {
        label: "run the soak",
        started_at: new Date(Date.now() - 125 * 60000).toISOString(),
        alive: true,
        last_output_at: "2026-11-08T10:00:00Z",
        last_lines: ["a", "b", "c"],
      },
    }),
  )
  const task = await screen.findByRole("group", { name: "Background task" })
  expect(task.textContent).toContain("A background task is still running.")
  expect(task.textContent).toContain("run the soak")
  expect(task.textContent).toContain("Running for 2h 5m")
  expect(task.textContent).toContain("process alive")
  expect(task.textContent).toContain("last output")
  expect(within(task).getByLabelText("Last output").textContent).toBe("a\nb\nc")
})

test("a gate card without a task shows its real cause and no task", async () => {
  renderTicket(() =>
    repair(["retry"], {
      pause_reason: "background-task-gate",
      pause_text: "Uncommitted work with 2 commits ahead.",
    }),
  )
  const c = await screen.findByRole("group", { name: "Action card" })
  expect(c.textContent).toContain("Uncommitted work with 2 commits ahead.")
  expect(screen.queryByRole("group", { name: "Background task" })).toBeNull()
  expect(c.textContent).not.toContain("still running")
})

test("a prompted Resume shows that the ticket stays paused", async () => {
  const body = {
    data: {
      ticket: "12",
      status: "prompted",
      message: "the agent is still running and was asked to continue",
    },
  }
  stubFetch(vi.fn(async () => new Response(JSON.stringify(body))))
  renderTicket(() => repair(["resume"], { pause_reason: "zero-commit" }))
  fireEvent.click(await screen.findByRole("button", { name: "Resume" }))
  expect(
    await screen.findByText(
      "Sent to the running agent. The ticket stays paused until it finishes.",
    ),
  ).toBeTruthy()
})

const oneOff = (status: string): TicketDetail => ({
  ...plain,
  address: "proj:one-offs/12",
  epic: "proj:one-offs",
  status,
})

test.each(["open", "claimed", "paused"])("a %s One-off shows Cancel", async (status) => {
  renderTicket(() => oneOff(status))
  expect(await screen.findByRole("button", { name: "Cancel" })).toBeTruthy()
})

test.each([
  ["a done One-off", oneOff("done")],
  ["a cancelled One-off", oneOff("cancelled")],
  ["an epic ticket", { ...plain, status: "claimed" } as TicketDetail],
])("%s shows no Cancel", async (_name, detail) => {
  renderTicket(() => detail)
  await screen.findByRole("heading", { level: 1 })
  expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull()
})

test("confirming Cancel posts the address", async () => {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify({})))
  stubFetch(fetchMock)
  renderTicket(() => oneOff("claimed"))
  fireEvent.click(await screen.findByRole("button", { name: "Cancel" }))
  expect(screen.getByText(/later runs of that Schedule may start/)).toBeTruthy()
  expect(fetchMock).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Cancel run" }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalled())
  expect(posted(fetchMock)[0]).toEqual([
    "/v1/tickets/cancel",
    { address: "proj:one-offs/12", state: "" },
  ])
})

test("a refused Cancel shows its reason", async () => {
  const refusal = { refused: true, reason: "ticket-done", message: "proj:one-offs/12 is done" }
  stubFetch(vi.fn(async () => new Response(JSON.stringify(refusal))))
  renderTicket(() => oneOff("open"))
  fireEvent.click(await screen.findByRole("button", { name: "Cancel" }))
  fireEvent.click(screen.getByRole("button", { name: "Cancel run" }))
  expect((await screen.findByRole("alert")).textContent).toBe("proj:one-offs/12 is done")
})
