import { useState } from "react"
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { PostAnswer } from "../design/prompt"
import {
  CardSourceClaudeDialog,
  CardSourceCodex,
  CardSourcePermission,
  type QuestionCard,
  SessionChatResponse,
  SessionFailure,
  SessionInfo,
  SessionResult,
} from "../generated/server"
import { roundCards } from "../design/roundFixture"
import { renderThemed } from "../ui/render"
import { AgentChat } from "./AgentChat"

afterEach(() => {
  cleanup()
  localStorage.clear()
})

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
  ...over,
})

const chat = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [
    {
      id: "m1",
      role: "agent",
      kind: "text",
      text: "hello there",
      at: new Date().toISOString(),
    },
  ],
  questions: [],
  background: [],
  ...over,
})

function render(
  s: SessionInfo,
  c: SessionChatResponse,
  post?: PostAnswer,
  refetch: () => void = () => {},
) {
  const state = {
    snapshot: { sessions: [s] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  // One mounted tree whose chat changes in place, so a new card id exercises the key and not a fresh mount.
  let setChat: (next: SessionChatResponse) => void = () => {}
  function Holder() {
    const [now, set] = useState(c)
    setChat = set
    return <AgentChat session={s} chat={now} refetch={refetch} post={post} />
  }
  const router = createMemoryRouter([{ path: "/", element: <Holder /> }])
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return { rerender: (next: SessionChatResponse) => act(() => setChat(next)) }
}

test("a busy session shows a plain Working row", async () => {
  render(session({ status: { state: "working" } }), chat())
  expect(await screen.findByText("Working")).toBeTruthy()
})

test("a busy session names the tool in flight", async () => {
  render(
    session({ status: { state: "working" } }),
    chat({
      current: { kind: "shell", label: "make test" },
      others: [{ kind: "search", label: "foo" }],
    }),
  )
  expect((await screen.findByRole("status")).textContent).toMatch(
    /Working · running shell: make test/,
  )
  expect(screen.getByRole("button", { name: "(+1 more)" })).toBeTruthy()
})

const round = (title = "Which way?") => [
  {
    label: "1",
    title,
    body: title,
    kind: "options" as const,
    options: [{ id: "a", label: "Alpha" }],
  },
]
const submitButton = () => screen.getByRole("button", { name: "Submit" }) as HTMLButtonElement

test("a card shows the sheet, and Submit waits for every answer", async () => {
  render(session(), chat({ questions: roundCards(round()) }))
  expect(await screen.findByText("Which way?")).toBeTruthy()
  expect(screen.queryByRole("group", { name: "View" })).toBeNull()
  expect(submitButton().disabled).toBe(true)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  expect(submitButton().disabled).toBe(false)
})

test("Huh? shows on a printed round only", async () => {
  const cards = roundCards(round())
  render(session(), chat({ questions: cards }))
  expect(await screen.findByRole("button", { name: "Huh?" })).toBeTruthy()
  cleanup()
  render(session(), chat({ questions: cards.map((c) => ({ ...c, source: "ticket-ask" })) }))
  expect(await screen.findByText("Which way?")).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Huh?" })).toBeNull()
})

test("a printed round card shows a note button beside Huh?", async () => {
  render(session(), chat({ questions: roundCards(round()) }))
  expect(await screen.findByRole("button", { name: "Huh?" })).toBeTruthy()
  expect(screen.getByRole("button", { name: /^Note on question/ })).toBeTruthy()
})

test("a multi-select card posts option ids, and the Other row posts other with its text", async () => {
  const post = vi.fn(async () => ({ refused: false }) as SessionResult)
  const card = sourceCard("claude-dialog", {}, true)
  card.questions[0].kind = "multi-select"
  render(session(), chat({ questions: [card] }), post)
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(screen.getByRole("button", { name: /Other/ }))
  fireEvent.change(screen.getByRole("textbox", { name: /Other/ }), {
    target: { value: "mine" },
  })
  fireEvent.click(submitButton())
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({
      answers: [expect.objectContaining({ choices: ["1", "other"], text: "mine" })],
    }),
  )
})

test("Submit posts the structured answer", async () => {
  const post = vi.fn(async () => ({ refused: false }) as SessionResult)
  const cards = roundCards(round())
  render(session(), chat({ questions: cards }), post)
  await screen.findByText("Which way?")
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(submitButton())
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({
      id: "s-a",
      source: "round",
      card_id: cards[0].id,
      answers: [expect.objectContaining({ choice: "a" })],
      last_message: "m1",
    }),
  )
})

test("a stale refusal shows its message and keeps the draft", async () => {
  const post = vi.fn(
    async () =>
      ({
        refused: true,
        reason: "stale",
        message: "The question changed.",
      }) as SessionResult,
  )
  const refetch = vi.fn()
  render(session(), chat({ questions: roundCards(round()) }), post, refetch)
  await screen.findByText("Which way?")
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(submitButton())
  expect((await screen.findByRole("alert")).textContent).toBe("The question changed.")
  expect(refetch).toHaveBeenCalled()
  expect(submitButton().disabled).toBe(false)
})

test("a new card id drops the draft", async () => {
  const state = { s: session(), post: vi.fn() }
  const first = roundCards(round())
  const view = render(state.s, chat({ questions: first }))
  await screen.findByText("Which way?")
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  expect(submitButton().disabled).toBe(false)
  view.rerender(chat({ questions: roundCards(round("Another way?")) }))
  await screen.findByText("Another way?")
  expect(submitButton().disabled).toBe(true)
})

const sourceCard = (
  source: string,
  over: Partial<QuestionCard> = {},
  text = false,
): QuestionCard => ({
  source,
  id: `${source}-1`,
  questions: [
    {
      kind: "options",
      title: "Pick one?",
      body: "The plan text",
      options: [
        { id: "1", label: "Alpha" },
        ...(text ? [{ id: "other", label: "Other", text: true }] : []),
      ],
    },
  ],
  ...over,
})

const buttonNames = () =>
  Array.from(document.querySelectorAll("button")).map((b) => b.textContent ?? "")
const has = (name: string) => buttonNames().includes(name)

test.each([
  ["a hook dialog", sourceCard("claude-dialog"), { release: true, dismiss: true, note: true }],
  [
    "a keystroke-fallback dialog",
    sourceCard("claude-dialog", { fallback: true }),
    { release: false, dismiss: true, note: false },
  ],
  ["a permission card", sourceCard("permission"), { release: false, dismiss: true, note: false }],
  [
    "a Codex question",
    sourceCard("codex-question"),
    { release: false, dismiss: false, note: true },
  ],
])("%s shows exactly its actions", async (_name, card, want) => {
  render(session(), chat({ questions: [card] }))
  await screen.findByText(/Pick one\?/)
  expect(has("Answer in the terminal")).toBe(want.release)
  expect(has("Dismiss")).toBe(want.dismiss)
  expect(!!screen.queryByRole("button", { name: /^Note on question/ })).toBe(want.note)
})

test("Dismiss posts an answer with no pick", async () => {
  const post = vi.fn(async () => ({ refused: false }) as SessionResult)
  const refetch = vi.fn()
  render(session(), chat({ questions: [sourceCard("permission")] }), post, refetch)
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: "Dismiss" }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({
      source: "permission",
      card_id: "permission-1",
      answers: [],
    }),
  )
  expect(refetch).toHaveBeenCalled()
})

test("Answer in the terminal posts a release", async () => {
  const post = vi.fn(async () => ({ refused: false }) as SessionResult)
  render(session(), chat({ questions: [sourceCard("claude-dialog")] }), post)
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: "Answer in the terminal" }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({ source: "claude-dialog", release: true }),
  )
})

test("a plan card shows its plan text and a text row that needs text", async () => {
  render(session(), chat({ questions: [sourceCard("permission", {}, true)] }))
  expect(await screen.findByText("The plan text")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: /Other/ }))
  expect(submitButton().disabled).toBe(true)
  fireEvent.change(screen.getByRole("textbox", { name: /Other/ }), {
    target: { value: "change it" },
  })
  expect(submitButton().disabled).toBe(false)
})

test("a Codex answer that stopped part way disables Submit and points to Screen", async () => {
  const post = vi.fn(
    async () =>
      ({
        refused: true,
        reason: "codex-stopped",
        message: "x",
      }) as SessionResult,
  )
  render(
    session({ harness: "codex" }),
    chat({
      questions: [sourceCard("codex-question", { id: "call_stopped_once" })],
    }),
    post,
  )
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(submitButton())
  expect((await screen.findByRole("alert")).textContent).toMatch(/Screen/)
  expect(submitButton().disabled).toBe(true)
})

test("a stopped Codex card keeps Submit disabled across a remount, and a new call re-enables it", async () => {
  const post = vi.fn(
    async () =>
      ({
        refused: true,
        reason: "codex-stopped",
        message: "x",
      }) as SessionResult,
  )
  const stopped = sourceCard("codex-question", { id: "call_stop" })
  render(session({ harness: "codex" }), chat({ questions: [stopped] }), post)
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(submitButton())
  await screen.findByRole("alert")
  cleanup()
  render(session({ harness: "codex" }), chat({ questions: [stopped] }), post)
  await screen.findByText(/Pick one\?/)
  // The draft keeps Alpha chosen, so only the stopped state can disable Submit.
  expect(submitButton().disabled).toBe(true)
  cleanup()
  render(
    session({ harness: "codex" }),
    chat({
      questions: [sourceCard("codex-question", { id: "call_new" })],
    }),
    post,
  )
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  expect(submitButton().disabled).toBe(false)
})

test("a Codex card shown before the overlay exists says to retry and keeps Submit", async () => {
  const post = vi.fn(
    async () =>
      ({
        refused: true,
        reason: "codex-not-ready",
        message: "x",
      }) as SessionResult,
  )
  render(session({ harness: "codex" }), chat({ questions: [sourceCard("codex-question")] }), post)
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(submitButton())
  expect((await screen.findByRole("alert")).textContent).toMatch(/not ready.*Retry/)
  expect(submitButton().disabled).toBe(false)
})

test("a keystroke-fallback refusal shows the server's text", async () => {
  const post = vi.fn(
    async () =>
      ({
        refused: true,
        reason: "bad-request",
        message: "the dialog on screen changed",
      }) as SessionResult,
  )
  render(session(), chat({ questions: [sourceCard("claude-dialog", { fallback: true })] }), post)
  await screen.findByText(/Pick one\?/)
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }))
  fireEvent.click(submitButton())
  expect((await screen.findByRole("alert")).textContent).toBe("the dialog on screen changed")
})

test("a card the next read no longer lists disappears", async () => {
  const view = render(session(), chat({ questions: [sourceCard("claude-dialog")] }))
  await screen.findByText(/Pick one\?/)
  view.rerender(chat({ questions: [] }))
  expect(screen.queryByText(/Pick one\?/)).toBeNull()
  expect(screen.queryByRole("button", { name: "Submit" })).toBeNull()
})

test("the banner still shows with a card visible", async () => {
  const failure = {
    kind: "stalled",
    label: "Failure stalled",
    text: "",
  } as SessionFailure
  render(session({ failure }), chat({ questions: roundCards(round()) }))
  await screen.findByText("Which way?")
  expect(screen.getByText("Failure stalled")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Screen" })).toBeTruthy()
})

test.each([
  [CardSourceClaudeDialog, false],
  [CardSourcePermission, false],
  [CardSourceCodex, true],
])("a blocked notice beside a %s card shows: %s", async (source, shown) => {
  const failure = { kind: "blocked", label: "Failure blocked", text: "" } as SessionFailure
  render(session({ failure }), chat({ questions: [sourceCard(source)] }))
  await screen.findByText(/Pick one\?/)
  expect(!!screen.queryByText("Failure blocked")).toBe(shown)
})

test("a gx-raised Actionable shows no gx stopped", async () => {
  render(session({ actionable: { gx_raised: true, reason: "never started" } }), chat())
  await screen.findByText("hello there")
  expect(screen.queryByLabelText("gx stopped")).toBeNull()
})

const BUTTONS: [string, string[]][] = [
  ["crashed", []],
  ["ended", []],
  ["auth", ["Retry"]],
  ["stalled", ["Screen"]],
  ["blocked", ["Screen"]],
  ["rate-limit", []],
]

test.each(BUTTONS)("the %s failure notice has the buttons %j", async (kind, expected) => {
  const failure = {
    kind,
    label: `Failure ${kind}`,
    text: "",
  } as SessionFailure
  render(session({ failure }), chat())
  await screen.findByText(`Failure ${kind}`)
  const region = screen.getByRole("region", { name: "Session failure" })
  const names = Array.from(region.querySelectorAll("button")).map((b) => b.textContent)
  expect(names).toEqual(expected)
})

test("a tools entry shows its count and each call", async () => {
  const at = new Date().toISOString()
  const tools = {
    id: "t1",
    role: "agent",
    kind: "tools",
    text: "",
    at,
    tool_count: 2,
    last_tool: "Read",
    calls: ["Bash: ls -la", "Read: /x/a.go"],
  }
  render(session(), chat({ messages: [tools] }))
  expect(await screen.findByText(/2 tool calls · last: Read/)).toBeTruthy()
  expect(screen.getByText("Bash: ls -la")).toBeTruthy()
  expect(screen.getByText("Read: /x/a.go")).toBeTruthy()
})

test("a prompt with images shows a grid above its text", async () => {
  const m = {
    id: "p1",
    role: "person",
    kind: "prompt",
    text: "why?",
    at: new Date().toISOString(),
    images: [{ id: "a" }, { id: "b" }],
  }
  render(session(), chat({ messages: [m] }))
  expect(await screen.findByText("why?")).toBeTruthy()
  expect(screen.getAllByRole("button", { name: /^Image \d of 2$/ })).toHaveLength(2)
})
