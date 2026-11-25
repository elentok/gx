import { act, cleanup, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { SessionChatResponse, SessionInfo, SessionSubagent } from "../generated/server"
import { renderThemed } from "../ui/render"
import { SubagentMissing, SubagentPage } from "./SubagentPage"

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  name: "Main agent",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
  ...over,
})

const chat = (
  sub: Partial<SessionSubagent>,
  over: Partial<SessionChatResponse> = {},
): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [
    { id: "m1", role: "agent", kind: "text", text: "looking around", at: "2026-01-01T00:00:00Z" },
  ],
  questions: [],
  subagent: {
    call_id: "call-1",
    root_session_id: "s-a",
    title: "Explore the repo",
    status: "done",
    prompt: "Find the entry point",
    ...sub,
  },
  ...over,
})

const show = (c: SessionChatResponse, s = session()) =>
  renderThemed(<SubagentPage session={s} chat={c} />)

test("a running subagent shows its header, the working row and no result card", () => {
  show(
    chat(
      {
        status: "running",
        type: "Explore",
        model: "opus",
        started_at: new Date(Date.now() - 65_000).toISOString(),
        tokens: 4200,
        tool_calls: 7,
        result: undefined,
      },
      { current: { kind: "shell", label: "go test ./..." } },
    ),
  )
  expect(screen.getByRole("heading", { name: "Explore the repo" })).toBeTruthy()
  for (const t of ["Explore", "Running", "opus", "4.2k", "7", "Tool calls", "1m 05s"])
    expect(screen.getByText(t)).toBeTruthy()
  expect(screen.getByText(/Working/)).toBeTruthy()
  expect(screen.getByText(/go test \.\/\.\.\./)).toBeTruthy()
  expect(screen.queryByText("Result handed back to the parent")).toBeNull()
})

test("a done subagent shows one green result card", () => {
  show(
    chat({
      result: "The entry point is main.go",
      started_at: "2026-01-01T00:00:00Z",
      ended_at: "2026-01-01T00:02:00Z",
    }),
  )
  expect(screen.getByText("Done")).toBeTruthy()
  expect(screen.getByText("2m 00s")).toBeTruthy()
  expect(screen.getAllByText("Result handed back to the parent")).toHaveLength(1)
  expect(screen.getByText("The entry point is main.go")).toBeTruthy()
})

test("a stopped subagent mutes its last message and shows no handback label", () => {
  show(chat({ status: "stopped", result: "I got as far as main.go" }))
  expect(screen.getByText("Stopped")).toBeTruthy()
  expect(screen.getByText("I got as far as main.go")).toBeTruthy()
  expect(screen.queryByText("Result handed back to the parent")).toBeNull()
})

test("a running subagent shows no result card even when the wire carries a result", () => {
  show(chat({ status: "running", result: "a message that is not a result" }))
  expect(document.querySelector('[data-part="subagent-result"]')).toBeNull()
})

test("a stopped subagent without a last message has no result card", () => {
  show(chat({ status: "stopped" }))
  expect(document.querySelector('[data-part="subagent-result"]')).toBeNull()
})

test("absent fields leave out exactly their chips and stats", () => {
  show(chat({}))
  for (const t of ["Model", "Time", "Tokens", "Tool calls"])
    expect(screen.queryByText(t)).toBeNull()
  expect(screen.queryByText("Explore")).toBeNull()
  expect(screen.getByText("Done")).toBeTruthy()
})

test("a Codex child with no prompt reads Prompt not readable and has no type chip", () => {
  show(chat({ prompt: undefined, type: undefined, model: "gpt-5" }), session({ harness: "codex" }))
  expect(screen.getByText("Prompt not readable")).toBeTruthy()
  expect(screen.getByText("Prompt from the parent agent")).toBeTruthy()
  expect(document.querySelectorAll('[data-tone="muted"]')).toHaveLength(0)
  expect(screen.getByText("Done")).toBeTruthy()
})

test("the prompt is shown when it is readable", () => {
  show(chat({}))
  expect(screen.getByText("Find the entry point")).toBeTruthy()
  expect(screen.queryByText("Prompt not readable")).toBeNull()
})

test("the breadcrumb names the session and the back link goes to the parent chat", () => {
  show(chat({}))
  expect(screen.getByRole("link", { name: "Main agent" }).getAttribute("href")).toBe("#/agents/s-a")
  expect(screen.getByRole("link", { name: "← Back to parent chat" }).getAttribute("href")).toBe(
    "#/agents/s-a",
  )
})

test("a nested subagent goes back to its parent subagent and the breadcrumb links each level", () => {
  show(
    chat({
      call_id: "call-2",
      title: "Inner",
      parent_call_id: "call-1",
      chain: [
        { call_id: "call-0", title: "Outer" },
        { call_id: "call-1", title: "Middle" },
      ],
    }),
  )
  expect(screen.getByRole("link", { name: "← Back to parent chat" }).getAttribute("href")).toBe(
    "#/agents/s-a/subagents/call-1",
  )
  expect(screen.getByRole("link", { name: "Main agent" }).getAttribute("href")).toBe("#/agents/s-a")
  expect(screen.getByRole("link", { name: "Outer" }).getAttribute("href")).toBe(
    "#/agents/s-a/subagents/call-0",
  )
  expect(screen.getByRole("link", { name: "Middle" }).getAttribute("href")).toBe(
    "#/agents/s-a/subagents/call-1",
  )
  expect(screen.queryByRole("link", { name: "Inner" })).toBeNull()
})

test("the breadcrumb falls back to the session id", () => {
  show(chat({}), session({ name: undefined }))
  expect(screen.getByRole("link", { name: "s-a" })).toBeTruthy()
})

test("the page has no composer", () => {
  show(chat({ status: "running" }))
  expect(screen.queryByRole("textbox")).toBeNull()
  expect(screen.queryByRole("button", { name: /send/i })).toBeNull()
})

test("Time ticks while running and stops after unmount", () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date("2026-01-01T00:01:00Z"))
  const { unmount } = show(chat({ status: "running", started_at: "2026-01-01T00:00:00Z" }))
  expect(screen.getByText("1m 00s")).toBeTruthy()
  act(() => {
    vi.advanceTimersByTime(5000)
  })
  expect(screen.getByText("1m 05s")).toBeTruthy()
  unmount()
  expect(vi.getTimerCount()).toBe(0)
})

const gone = (over: Partial<SessionSubagent>) =>
  chat({ status: "not_available", unavailable: "gone", type: "Explore", ...over }, { messages: [] })

test("Starting shows the prompt of a Claude subagent", () => {
  show(chat({ status: "running", unavailable: "starting" }, { messages: [] }))
  expect(screen.getByText("Starting…")).toBeTruthy()
  expect(screen.getByText("Prompt from the parent agent")).toBeTruthy()
  expect(screen.getByText("Find the entry point")).toBeTruthy()
})

test("Starting a Codex child shows Prompt not readable", () => {
  show(chat({ status: "running", unavailable: "starting", prompt: undefined }, { messages: [] }))
  expect(screen.getByText("Starting…")).toBeTruthy()
  expect(screen.getByText("Prompt not readable")).toBeTruthy()
})

test("A gone file shows Not available, the note and the Claude prompt", () => {
  show(gone({}))
  expect(screen.getByText("Not available")).toBeTruthy()
  expect(screen.getByText("Transcript no longer available")).toBeTruthy()
  expect(screen.getByText("Find the entry point")).toBeTruthy()
  expect(screen.getByRole("heading", { name: "Explore the repo" })).toBeTruthy()
})

test("A compressed child says reading is not supported yet and has no Decode button", () => {
  show(gone({ unavailable: "compressed", prompt: undefined }))
  expect(screen.getByText("Transcript no longer available")).toBeTruthy()
  expect(screen.getByText(/not supported yet/)).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Decode" })).toBeNull()
})

const parentGone = (unavailable: string, archived = false) =>
  show(
    chat(
      { status: "not_available", unavailable, title: undefined, prompt: undefined },
      { messages: [], archived },
    ),
  )

test("An unreadable parent shows only the note and the back link", () => {
  parentGone("gone")
  expect(screen.getByText("Transcript no longer available")).toBeTruthy()
  expect(screen.getByRole("link", { name: /Back to parent chat/ }).getAttribute("href")).toBe(
    "#/agents/s-a",
  )
  expect(screen.queryByRole("heading")).toBeNull()
  expect(screen.queryByText("Not available")).toBeNull()
  expect(screen.queryByText("Prompt from the parent agent")).toBeNull()
})

test("An archived parent points to the chat page and never says not supported yet", () => {
  parentGone("compressed", true)
  expect(screen.getByText(/parent session is archived/)).toBeTruthy()
  expect(screen.getByRole("link", { name: "chat page" }).getAttribute("href")).toBe("#/agents/s-a")
  expect(screen.queryByText(/not supported yet/)).toBeNull()
  expect(screen.queryByText("Prompt from the parent agent")).toBeNull()
})

test("A compressed child with a title is not mistaken for an archived parent", () => {
  show(gone({ unavailable: "compressed", title: "Explore the repo" }))
  expect(screen.queryByText(/parent session is archived/)).toBeNull()
  expect(screen.getByRole("heading", { name: "Explore the repo" })).toBeTruthy()
})

test("A 404 shows the server's message and the back link", () => {
  renderThemed(<SubagentMissing sessionId="s-a" message="no such subagent" />)
  expect(screen.getByText("no such subagent")).toBeTruthy()
  expect(screen.getByRole("link", { name: /Back to parent chat/ })).toBeTruthy()
})

test("An unknown session's message never reads no such subagent", () => {
  renderThemed(<SubagentMissing sessionId="s-x" message="no session s-x" />)
  expect(screen.getByText("no session s-x")).toBeTruthy()
  expect(screen.queryByText(/no such subagent/)).toBeNull()
})
