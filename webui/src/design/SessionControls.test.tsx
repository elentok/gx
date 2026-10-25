import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import {
  ConnectionContext,
  type Connection,
  type ConnectionState,
} from "../connection";
import type {
  EpicDetail,
  SessionChatResponse,
  SessionInfo,
  SessionPromptRequest,
  SessionResult,
} from "../generated/server";
import { appRoutes, type RouteFetchers } from "../routes";
import { renderThemed } from "../ui/render";
import { stateToken } from "./open";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const session = (id: string, over: Partial<SessionInfo> = {}): SessionInfo => ({
  id,
  name: id,
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: "2026-10-23T10:00:00.123456789Z",
  run_started_at: "2026-10-23T10:00:00.123456789Z",
  unread: {},
  actionable: {},
  ...over,
});

const detailOf = (over: Partial<EpicDetail> = {}): EpicDetail => ({
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
  steps: [
    { step: "chart", label: "Chart", state: "current" },
    { step: "done", label: "Done", state: "todo" },
  ],
  ...over,
});

const chat: SessionChatResponse = {
  conversation_id: "c",
  messages: [],
  round: null,
  background: [],
  truncated: false,
} as unknown as SessionChatResponse;
const ok: SessionResult = { session: session("new") };

function render(
  path: string,
  sessions: SessionInfo[],
  over: Partial<RouteFetchers> = {},
  detail = detailOf(),
) {
  const state = {
    snapshot: { sessions },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
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
      epic: async () => ({ kind: "ok", detail }),
      chat: async () => ({ kind: "ok", detail: chat }),
      ...over,
    }),
    { initialEntries: [path] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

test("opening the Design page with no session writes nothing and offers Start", async () => {
  const openDesign = vi.fn(async () => ok);
  const prompt = vi.fn();
  render("/d/gx:a", [], { openDesign, prompt });
  await screen.findByRole("button", { name: "Start design session" });
  expect(openDesign).not.toHaveBeenCalled();
  expect(prompt).not.toHaveBeenCalled();
});

test("a double click on Start opens once", async () => {
  const openDesign = vi.fn(() => new Promise<SessionResult>(() => {}));
  render("/d/gx:a", [], { openDesign });
  const start = await screen.findByRole("button", {
    name: "Start design session",
  });
  fireEvent.click(start);
  fireEvent.click(start);
  expect(openDesign).toHaveBeenCalledTimes(1);
  expect(openDesign).toHaveBeenCalledWith({ epic: "gx:a" });
});

test("a refused Start shows the message", async () => {
  render("/d/gx:a", [], {
    openDesign: async () => ({ refused: true, message: "no trunk checkout" }),
  });
  fireEvent.click(
    await screen.findByRole("button", { name: "Start design session" }),
  );
  expect((await screen.findByRole("alert")).textContent).toBe(
    "no trunk checkout",
  );
});

test("the map epic page reads Start without a session and goes to the Design after it", async () => {
  const openDesign = vi.fn(async () => ok);
  const router = render("/e/gx:a", [], { openDesign });
  fireEvent.click(
    await screen.findByRole("button", { name: "Start design session" }),
  );
  await waitFor(() => expect(router.state.location.pathname).toBe("/d/gx:a"));
  expect(openDesign).toHaveBeenCalledWith({ epic: "gx:a" });
});

test("the map epic page reads Open with a session and only navigates", async () => {
  const openDesign = vi.fn(async () => ok);
  const router = render(
    "/e/gx:a",
    [session("s1", { status: { state: "exited" } })],
    { openDesign },
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "Open design session" }),
  );
  await waitFor(() => expect(router.state.location.pathname).toBe("/d/gx:a"));
  expect(openDesign).not.toHaveBeenCalled();
});

test("a Done Design has no session button on its epic page", async () => {
  render(
    "/e/gx:a",
    [],
    {},
    detailOf({ steps: [{ step: "done", label: "Done", state: "done" }] }),
  );
  await screen.findByText("No tickets yet");
  expect(screen.queryByRole("button", { name: /design session/ })).toBeNull();
});

test("New session confirms, then opens new with the state token", async () => {
  const s = session("s1");
  const openDesign = vi.fn(async () => ok);
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  render("/d/gx:a", [s], { openDesign });
  fireEvent.click(await screen.findByRole("button", { name: "New session" }));
  expect(confirm).toHaveBeenCalledWith(
    "Stop the current session and start a new one?",
  );
  expect(openDesign).toHaveBeenCalledWith({
    epic: "gx:a",
    new: true,
    state: stateToken(s),
  });
});

test("declining the confirm sends nothing", async () => {
  const openDesign = vi.fn(async () => ok);
  vi.spyOn(window, "confirm").mockReturnValue(false);
  render("/d/gx:a", [session("s1")], { openDesign });
  fireEvent.click(await screen.findByRole("button", { name: "New session" }));
  expect(openDesign).not.toHaveBeenCalled();
});

test("the state token has the server's shape", () => {
  expect(stateToken(session("s1"))).toBe("s1:idle:1792749600123456789");
});

test("the N live menu switches the page and stops another session with a token", async () => {
  // The older run flipped state last; the newer run start still wins.
  const older = session("old", {
    run_started_at: "2026-10-23T09:00:00Z",
    status_changed_at: "2026-10-23T11:00:00Z",
  });
  const newer = session("new", {
    run_started_at: "2026-10-23T10:00:00Z",
    status_changed_at: "2026-10-23T10:00:00Z",
  });
  const stop = vi.fn(async () => ({}) as SessionResult);
  const chatFor = vi.fn(async () => ({ kind: "ok" as const, detail: chat }));
  render("/d/gx:a", [older, newer], { stop, chat: chatFor });
  await waitFor(() => expect(chatFor).toHaveBeenCalledWith("new"));
  fireEvent.click(await screen.findByRole("button", { name: /2 live/ }));
  fireEvent.click(screen.getByRole("menuitem", { name: "old" }));
  await waitFor(() => expect(chatFor).toHaveBeenCalledWith("old"));
  fireEvent.click(screen.getByRole("button", { name: /2 live/ }));
  fireEvent.click(screen.getByRole("button", { name: "Stop new" }));
  expect(stop).toHaveBeenCalledWith({ id: "new", state: stateToken(newer) });
});

test("a send goes to the session on screen", async () => {
  // The older run flipped state last; the newer run start still wins.
  const older = session("old", {
    run_started_at: "2026-10-23T09:00:00Z",
    status_changed_at: "2026-10-23T11:00:00Z",
  });
  const newer = session("new", {
    run_started_at: "2026-10-23T10:00:00Z",
    status_changed_at: "2026-10-23T10:00:00Z",
  });
  const prompt = vi.fn(
    async (_body: SessionPromptRequest) => ({}) as SessionResult,
  );
  const withContinue = {
    ...chat,
    next: { action: { action: ["/clear", "/gx-design a"] } },
  } as SessionChatResponse;
  render("/d/gx:a", [older, newer], {
    prompt,
    chat: async () => ({ kind: "ok", detail: withContinue }),
  });
  fireEvent.click(await screen.findByRole("button", { name: /2 live/ }));
  fireEvent.click(screen.getByRole("menuitem", { name: "old" }));
  fireEvent.click(await screen.findByRole("button", { name: /Continue/ }));
  await waitFor(() => expect(prompt).toHaveBeenCalled());
  expect(prompt.mock.calls[0][0]).toMatchObject({ id: "old", continue: true });
});
