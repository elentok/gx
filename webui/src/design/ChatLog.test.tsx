import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionChatMessage, SessionChatResponse, SessionInfo } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchChat } from "./chat";

afterEach(cleanup);

const ago = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: ago(1),
  unread: {},
  actionable: {},
  ...over,
});

const msg = (over: Partial<SessionChatMessage>): SessionChatMessage => ({
  id: String(Math.random()),
  role: "agent",
  kind: "text",
  text: "",
  at: ago(1),
  ...over,
});

const chatOf = (messages: SessionChatMessage[], over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages,
  round: [],
  background: [],
  ...over,
});

const okChat = (c: SessionChatResponse): FetchChat => async () => ({ kind: "ok", detail: c });

function renderChat(sessions: SessionInfo[], fetchChat: FetchChat) {
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const listeners = new Set<(a?: string) => void>();
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: (a?: string) => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
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
            blocked_by_epics: [],
            picker_section: "in-progress",
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
          rows: [],
          queueable: false,
          queued: false,
          steps: [],
        },
      }),
      chat: fetchChat,
    }),
    { initialEntries: ["/d/gx:a"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { emit: (a?: string) => act(async () => listeners.forEach((l) => l(a))) };
}

test("each entry kind renders", async () => {
  renderChat(
    [session()],
    okChat(
      chatOf(
        [
          msg({ text: "Hello **bold**" }),
          msg({ role: "person", kind: "prompt", text: "do the thing" }),
          msg({ role: "person", kind: "command", text: "/gx-design" }),
          msg({ kind: "tools", tool_count: 12, last_tool: "Read" }),
          msg({ kind: "compacted" }),
        ],
        { truncated: true },
      ),
    ),
  );
  expect((await screen.findByText("bold")).tagName).toBe("STRONG");
  expect(screen.getByText("do the thing")).toBeTruthy();
  expect(screen.getByText("/gx-design")).toBeTruthy();
  expect(screen.getByText("· 12 tool calls")).toBeTruthy();
  expect(screen.getByText("Context compacted")).toBeTruthy();
  expect(screen.getByText("Earlier messages hidden")).toBeTruthy();
});

test("the tools line names the latest tool only while Working", async () => {
  const chat = okChat(chatOf([msg({ kind: "tools", tool_count: 3, last_tool: "Grep" })]));
  renderChat([session({ status: { state: "working" } })], chat);
  expect(await screen.findByText("· 3 tool calls, Grep")).toBeTruthy();
  cleanup();
  renderChat([session({ status: { state: "idle" } })], chat);
  expect(await screen.findByText("· 3 tool calls")).toBeTruthy();
});

test("the background strip lists running tasks", async () => {
  renderChat(
    [session()],
    okChat(
      chatOf([], {
        background: [
          { kind: "shell", label: "make web-test", started_at: ago(0) },
          { kind: "agent", label: "survey", started_at: ago(0) },
        ],
      }),
    ),
  );
  const strip = await screen.findByRole("list", { name: "Running in background" });
  expect(within(strip).getByText("shell: make web-test")).toBeTruthy();
  expect(within(strip).getByText("subagent: survey")).toBeTruthy();
});

test("an event for the session refetches and a new conversation replaces the chat", async () => {
  let n = 0;
  const fetchChat = vi.fn<FetchChat>(async () => ({
    kind: "ok",
    detail: ++n === 1 ? chatOf([msg({ text: "old talk" })]) : chatOf([msg({ text: "new talk" })], { conversation_id: "c2" }),
  }));
  const { emit } = renderChat([session()], fetchChat);
  await screen.findByText("old talk");
  await emit("s-other");
  expect(fetchChat).toHaveBeenCalledTimes(1);
  await emit("s-a");
  await screen.findByText("new talk");
  expect(screen.queryByText("old talk")).toBeNull();
});

test("a non-Claude session shows the fixed text and loads no chat", async () => {
  const fetchChat = vi.fn<FetchChat>(okChat(chatOf([])));
  renderChat([session({ harness: "codex" })], fetchChat);
  expect(await screen.findByText("Chat works for Claude sessions only. Open it in gx, Agents tab.")).toBeTruthy();
  expect(fetchChat).not.toHaveBeenCalled();
});

test("a Design with no session has an empty chat", async () => {
  const fetchChat = vi.fn<FetchChat>(okChat(chatOf([])));
  renderChat([], fetchChat);
  await screen.findByRole("region", { name: "Chat" });
  expect(fetchChat).not.toHaveBeenCalled();
});

test("the newest live session drives the chat, else the newest Exited one", async () => {
  const seen: string[] = [];
  const fetchChat: FetchChat = async (id) => {
    seen.push(id);
    return { kind: "ok", detail: chatOf([]) };
  };
  renderChat(
    [
      session({ id: "old-exited", status: { state: "exited" }, status_changed_at: ago(9) }),
      session({ id: "new-exited", status: { state: "exited" }, status_changed_at: ago(1) }),
    ],
    fetchChat,
  );
  await waitFor(() => expect(seen).toEqual(["new-exited"]));
});
