import { useState } from "react";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { PostAnswer } from "../design/prompt";
import type { SessionChatResponse, SessionFailure, SessionInfo, SessionResult } from "../generated/server";
import { roundCards } from "../design/roundFixture";
import { renderThemed } from "../ui/render";
import { AgentChat } from "./AgentChat";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

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
});

const chat = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [{ id: "m1", role: "agent", kind: "text", text: "hello there", at: new Date().toISOString() }],
  questions: [],
  background: [],
  ...over,
});

function render(s: SessionInfo, c: SessionChatResponse, post?: PostAnswer, refetch: () => void = () => {}) {
  const state = { snapshot: { sessions: [s] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  // One mounted tree whose chat changes in place, so a new card id exercises the key and not a fresh mount.
  let setChat: (next: SessionChatResponse) => void = () => {};
  function Holder() {
    const [now, set] = useState(c);
    setChat = set;
    return <AgentChat session={s} chat={now} refetch={refetch} post={post} />;
  }
  const router = createMemoryRouter([{ path: "/", element: <Holder /> }]);
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { rerender: (next: SessionChatResponse) => act(() => setChat(next)) };
}

test("a busy session shows a plain Working row", async () => {
  render(session({ status: { state: "working" } }), chat());
  expect(await screen.findByText("Working")).toBeTruthy();
});

test("a busy session names the tool in flight", async () => {
  render(
    session({ status: { state: "working" } }),
    chat({ current: { kind: "shell", label: "make test" }, others: [{ kind: "search", label: "foo" }] }),
  );
  expect((await screen.findByRole("status")).textContent).toMatch(/Working · running shell: make test/);
  expect(screen.getByRole("button", { name: "(+1 more)" })).toBeTruthy();
});

const round = (title = "Which way?") => [
  { label: "1", title, body: title, kind: "options" as const, options: [{ id: "a", label: "Alpha" }] },
];
const submitButton = () => screen.getByRole("button", { name: "Submit" }) as HTMLButtonElement;

test("a card shows the sheet, and Submit waits for every answer", async () => {
  render(session(), chat({ questions: roundCards(round()) }));
  expect(await screen.findByText("Which way?")).toBeTruthy();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
  expect(submitButton().disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }));
  expect(submitButton().disabled).toBe(false);
});

test("Submit posts the structured answer", async () => {
  const post = vi.fn(async () => ({ refused: false }) as SessionResult);
  const cards = roundCards(round());
  render(session(), chat({ questions: cards }), post);
  await screen.findByText("Which way?");
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }));
  fireEvent.click(submitButton());
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post).toHaveBeenCalledWith(
    expect.objectContaining({ id: "s-a", source: "round", card_id: cards[0].id, answers: [expect.objectContaining({ choice: "a" })], last_message: "m1" }),
  );
});

test("a stale refusal shows its message and keeps the draft", async () => {
  const post = vi.fn(async () => ({ refused: true, reason: "stale", message: "The question changed." }) as SessionResult);
  const refetch = vi.fn();
  render(session(), chat({ questions: roundCards(round()) }), post, refetch);
  await screen.findByText("Which way?");
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }));
  fireEvent.click(submitButton());
  expect((await screen.findByRole("alert")).textContent).toBe("The question changed.");
  expect(refetch).toHaveBeenCalled();
  expect(submitButton().disabled).toBe(false);
});

test("a new card id drops the draft", async () => {
  const state = { s: session(), post: vi.fn() };
  const first = roundCards(round());
  const view = render(state.s, chat({ questions: first }));
  await screen.findByText("Which way?");
  fireEvent.click(screen.getByRole("button", { name: /Alpha/ }));
  expect(submitButton().disabled).toBe(false);
  view.rerender(chat({ questions: roundCards(round("Another way?")) }));
  await screen.findByText("Another way?");
  expect(submitButton().disabled).toBe(true);
});

test("the banner still shows with a card visible", async () => {
  const failure = { kind: "stalled", label: "Failure stalled", text: "" } as SessionFailure;
  render(session({ failure }), chat({ questions: roundCards(round()) }));
  await screen.findByText("Which way?");
  expect(screen.getByText("Failure stalled")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Screen" })).toBeTruthy();
});

test("a gx-raised Actionable shows no gx stopped", async () => {
  render(session({ actionable: { gx_raised: true, reason: "never started" } }), chat());
  await screen.findByText("hello there");
  expect(screen.queryByLabelText("gx stopped")).toBeNull();
});

const BUTTONS: [string, string[]][] = [
  ["crashed", []],
  ["ended", []],
  ["auth", ["Retry"]],
  ["stalled", ["Screen"]],
  ["blocked", ["Screen"]],
  ["rate-limit", []],
];

test.each(BUTTONS)("the %s failure notice has the buttons %j", async (kind, expected) => {
  const failure = { kind, label: `Failure ${kind}`, text: "" } as SessionFailure;
  render(session({ failure }), chat());
  await screen.findByText(`Failure ${kind}`);
  const region = screen.getByRole("region", { name: "Session failure" });
  const names = Array.from(region.querySelectorAll("button")).map((b) => b.textContent);
  expect(names).toEqual(expected);
});

test("a tools entry shows its count and each call", async () => {
  const at = new Date().toISOString();
  const tools = { id: "t1", role: "agent", kind: "tools", text: "", at, tool_count: 2, last_tool: "Read", calls: ["Bash: ls -la", "Read: /x/a.go"] };
  render(session(), chat({ messages: [tools] }));
  expect(await screen.findByText(/2 tool calls · last: Read/)).toBeTruthy();
  expect(screen.getByText("Bash: ls -la")).toBeTruthy();
  expect(screen.getByText("Read: /x/a.go")).toBeTruthy();
});

test("a prompt with images shows a grid above its text", async () => {
  const m = { id: "p1", role: "person", kind: "prompt", text: "why?", at: new Date().toISOString(), images: [{ id: "a" }, { id: "b" }] };
  render(session(), chat({ messages: [m] }));
  expect(await screen.findByText("why?")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: /^Image \d of 2$/ })).toHaveLength(2);
});
