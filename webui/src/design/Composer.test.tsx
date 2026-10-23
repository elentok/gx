import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionChatResponse, SessionInfo, SessionResult } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchChat } from "./chat";
import type { PostPrompt } from "./prompt";

afterEach(cleanup);

const session = (state = "idle"): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state } as SessionInfo["status"],
  status_changed_at: new Date().toISOString(),
  unread: {},
  actionable: {},
});

const chatOf = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [{ id: "m9", role: "agent", kind: "text", text: "hi", at: new Date().toISOString() }],
  round: [],
  background: [],
  ...over,
});

const nextOf = (phase: string): SessionChatResponse["next"] => ({ phase, action: {} });

const ok: SessionResult = { session: session() };

function render(opts: { state?: string; chat?: SessionChatResponse; connected?: boolean; post?: PostPrompt; fetchChat?: FetchChat }) {
  const state = {
    snapshot: { sessions: [session(opts.state)] },
    connected: opts.connected ?? true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({
        epics: [
          { address: "gx:a", project: "gx", name: "a", kind: "map", state: "designing", done: 0, total: 0, queued: false, queueable: false, blocked_by_epics: [], picker_section: "in-progress" },
        ],
        follow_ups: [],
      }),
      epic: async () => ({
        kind: "ok",
        detail: { address: "gx:a", kind: "map", summary: "", done: 0, total: 0, agent_time: 0, cost_total: 0, rows: [], queueable: false, queued: false, steps: [] },
      }),
      chat: opts.fetchChat ?? (async () => ({ kind: "ok", detail: opts.chat ?? chatOf() })),
      prompt: opts.post,
    }),
    { initialEntries: ["/d/gx:a"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const openNote = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "Note" }));
  return (await screen.findByLabelText("Message")) as HTMLTextAreaElement;
};

test("a plain message posts its text with the last message id", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post });
  const box = await openNote();
  fireEvent.change(box, { target: { value: "hello there" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post).toHaveBeenCalledWith({ id: "s-a", text: "hello there", last_message: "m9" });
  await waitFor(() => expect(box.value).toBe(""));
});

test("the composer is disabled while the session is Working", async () => {
  render({ state: "working", post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("map") }) });
  const box = await openNote();
  expect(box.disabled).toBe(true);
  expect((screen.getByRole("button", { name: /Continue/ }) as HTMLButtonElement).disabled).toBe(true);
});

test("a stale refusal keeps the draft and refetches the chat", async () => {
  const post = vi.fn<PostPrompt>(async () => ({ refused: true, reason: "stale", message: "the conversation moved; review it first" }));
  const fetchChat = vi.fn<FetchChat>(async () => ({ kind: "ok", detail: chatOf() }));
  render({ post, fetchChat });
  const box = await openNote();
  const before = fetchChat.mock.calls.length;
  fireEvent.change(box, { target: { value: "keep me" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(box.value).toBe("keep me");
  await waitFor(() => expect(fetchChat.mock.calls.length).toBeGreaterThan(before));
});

test("another refusal shows its message and keeps the draft", async () => {
  const post = vi.fn<PostPrompt>(async () => ({ refused: true, reason: "text-too-long", message: "text is over 32768 bytes" }));
  render({ post });
  const box = await openNote();
  fireEvent.change(box, { target: { value: "long" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect((await screen.findByRole("alert")).textContent).toBe("text is over 32768 bytes");
  expect(box.value).toBe("long");
});

test.each(["map", "review"])("Continue after a %s run-end line sends the continue action", async (phase) => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ next: nextOf(phase) }) });
  fireEvent.click(await screen.findByRole("button", { name: new RegExp(`Continue · ${phase}`) }));
  await waitFor(() => expect(post).toHaveBeenCalledWith({ id: "s-a", text: "", last_message: "m9", continue: true }));
});

test("an implement run-end line shows no Continue", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("implement") }) });
  await screen.findByRole("button", { name: "Note" });
  expect(screen.queryByRole("button", { name: /Continue/ })).toBeNull();
});

test("while disconnected the page says so and the buttons are off", async () => {
  render({ connected: false, post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("map") }) });
  expect(((await screen.findByRole("button", { name: /Continue/ })) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
  expect((await openNote()).disabled).toBe(true);
});
