import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionInfo } from "../generated/server";
import type { SessionChatResponse } from "../generated/server";
import { appRoutes, type RouteFetchers } from "../routes";
import { renderThemed } from "../ui/render";
import { WideContext } from "../useWide";

export const session = (id: string, project: string, over: Partial<SessionInfo> = {}): SessionInfo => ({
  id,
  project,
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

export const focusPosts: { session: string }[] = [];

// Call once at the top of each AgentsPage test file: it registers the shared cleanup and fetch stub.
export function useAgentsPageHooks() {
  afterEach(cleanup);
  beforeEach(() => {
    localStorage.clear();
    focusPosts.length = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init: { body: string }) => {
        focusPosts.push(JSON.parse(init.body));
        return new Response("{}");
      }),
    );
  });
  afterEach(() => vi.unstubAllGlobals());
}

export function render(sessions: SessionInfo[] | undefined, path = "/agents", live = !!sessions, fetchers: RouteFetchers = {}, wide?: boolean) {
  let state = { snapshot: sessions && { sessions }, connected: true, loadedOnce: !!sessions, liveSnapshot: live, streams: 1, tick: 0 } as unknown as ConnectionState;
  const listeners = new Set<() => void>();
  const handlers = new Set<(a?: string) => void>();
  const conn = {
    clientId: "c1",
    getState: () => state,
    subscribe: (fn: () => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    onEvent: (fn: (a?: string) => void) => {
      handlers.add(fn);
      return () => handlers.delete(fn);
    },
    refresh: () => {},
  } as unknown as Connection;
  const router = createMemoryRouter(appRoutes(fetchers), { initialEntries: [path] });
  renderThemed(
    <WideContext.Provider value={wide}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </WideContext.Provider>,
  );
  const update = (patch: Partial<ConnectionState>) => {
    state = { ...state, ...patch } as unknown as ConnectionState;
    act(() => listeners.forEach((fn) => fn()));
  };
  const setSessions = (next: SessionInfo[]) => update({ snapshot: { sessions: next } } as Partial<ConnectionState>);
  const goLive = () => update({ liveSnapshot: true });
  const emit = (address: string) => act(() => handlers.forEach((fn) => fn(address)));
  return { router, setSessions, goLive, emit, update };
}

export const rows = () => screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("#/agents/"));
export const row = (name: string) => rows().find((r) => within(r).queryByText(name))!;

export const chat = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [{ id: "m1", role: "agent", kind: "text", text: "hello there", at: "2026-01-01T00:00:00Z" }],
  round: null,
  background: [],
  ...over,
});

export const chatOf = (c: SessionChatResponse) => ({ chat: async () => ({ kind: "ok" as const, detail: c }) });

export const switcherSessions = () => [
  session("s-1", "alpha", { name: "plain", dir: "/work/plain" }),
  session("s-2", "alpha", { name: "unread one", unread: { unread: true } }),
  session("s-3", "beta", { name: "needs you", ticket: "gx:m/07", actionable: { reason: "Pick one" } }),
  session("s-4", "beta", { name: "the design", design: true, dir: "/work/design" }),
];

export const openSwitcher = () => fireEvent.click(screen.getByRole("button", { name: /plain/ }));
export const dialog = () => within(screen.getByRole("dialog", { name: "Switch session" }));
export const optionNames = () => dialog().getAllByRole("option").map((o) => o.textContent);

// Answers the prompt POST with `answer`; everything else (focus reports) gets an empty object.
export const prompts: { id: string; text: string }[] = [];
export const stubPrompt = (answer: () => Response) =>
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: { body: string }) => {
      if (url !== "/v1/sessions/prompt") return new Response("{}");
      prompts.push(JSON.parse(init.body));
      return answer();
    }),
  );
export const accepted = () => new Response(JSON.stringify({ refused: false }));
export const refused = (reason: string, message?: string) => () => new Response(JSON.stringify({ refused: true, reason, message }));

export const composerBox = () => screen.findByRole("textbox", { name: "Message" }) as Promise<HTMLTextAreaElement>;
export const sendButton = () => screen.getByRole("button", { name: "Send" }) as HTMLButtonElement;
export const type = async (text: string) => fireEvent.change(await composerBox(), { target: { value: text } });

const entryTrees = [
  { project: "p", root: "/dev/p", worktrees: [] },
  { project: "q", root: "/dev/q", worktrees: [] },
];
export const entryOptions = { worktrees: async () => entryTrees, chat: async () => ({ kind: "not-found" as const }) };
export const formProject = async () => ((await screen.findByRole("combobox", { name: "Project" })) as HTMLSelectElement).value;
