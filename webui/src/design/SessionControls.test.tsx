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
import { WriteError } from "../useWrite";
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
  auto_advance: true,
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
  expect(await screen.findByText("no trunk checkout")).toBeTruthy();
});

test("Start is disabled while the write is pending, then a server error shows its text in a toast", async () => {
  let fail: (e: Error) => void = () => {};
  const openDesign = vi.fn(() => new Promise<SessionResult>((_, reject) => (fail = reject)));
  render("/d/gx:a", [], { openDesign });
  fireEvent.click(await screen.findByRole("button", { name: "Start design session" }));
  await waitFor(() =>
    expect((screen.getByRole("button", { name: "Start design session" }) as HTMLButtonElement).disabled).toBe(true),
  );
  fail(new WriteError("trunk is dirty, commit first"));
  expect(await screen.findByText("trunk is dirty, commit first")).toBeTruthy();
  await waitFor(() =>
    expect((screen.getByRole("button", { name: "Start design session" }) as HTMLButtonElement).disabled).toBe(false),
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

const openMenu = async () => fireEvent.click(await screen.findByRole("button", { name: "More actions" }));
const newSession = () => screen.findByRole("menuitem", { name: /New session/ });

test("New session confirms, then opens new with the state token", async () => {
  const s = session("s1");
  const openDesign = vi.fn(async () => ok);
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  render("/d/gx:a", [s], { openDesign });
  await openMenu();
  const item = await newSession();
  expect(item.textContent).toContain("Stop this session and start a fresh one");
  fireEvent.click(item);
  await waitFor(() =>
    expect(openDesign).toHaveBeenCalledWith({
      epic: "gx:a",
      new: true,
      state: stateToken(s),
    }),
  );
  expect(confirm).toHaveBeenCalledWith(
    "Stop the current session and start a new one?",
  );
});

test("declining the confirm sends nothing", async () => {
  const openDesign = vi.fn(async () => ok);
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  render("/d/gx:a", [session("s1")], { openDesign });
  await openMenu();
  fireEvent.click(await newSession());
  await waitFor(() => expect(confirm).toHaveBeenCalled());
  expect(openDesign).not.toHaveBeenCalled();
});

test("New session on an exited session asks nothing and sends no state token", async () => {
  const openDesign = vi.fn(async () => ok);
  const confirm = vi.spyOn(window, "confirm");
  render("/d/gx:a", [session("s1", { status: { state: "exited" } })], { openDesign });
  await openMenu();
  const item = await newSession();
  expect(item.textContent).toContain("Start a fresh session");
  fireEvent.click(item);
  await waitFor(() => expect(openDesign).toHaveBeenCalledWith({ epic: "gx:a", new: true }));
  expect(confirm).not.toHaveBeenCalled();
});

test("the state token has the server's shape", () => {
  expect(stateToken(session("s1"))).toBe("s1:idle:1792749600123456789");
});

test("the menu lists Live sessions only with 2 or more, and has none with one", async () => {
  render("/d/gx:a", [session("only")]);
  await openMenu();
  await newSession();
  expect(screen.queryByText(/Live sessions/)).toBeNull();
});

test("the Live sessions list switches the page and stops another session with a token", async () => {
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
  await openMenu();
  expect(await screen.findByText("Live sessions · 2")).toBeTruthy();
  expect(screen.getByRole("menuitem", { name: "new" }).getAttribute("aria-current")).toBe("true");
  expect(screen.queryByRole("button", { name: "Stop new" })).toBeNull();
  fireEvent.click(screen.getByRole("menuitem", { name: "old" }));
  await waitFor(() => expect(chatFor).toHaveBeenCalledWith("old"));
  await openMenu();
  fireEvent.click(await screen.findByRole("button", { name: "Stop new" }));
  await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "new", state: stateToken(newer) }));
});

const phone = () => vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));

test("on a phone the ⋯ menu is a bottom sheet with a grab handle and the desktop items", async () => {
  phone();
  try {
    const s = session("s1");
    const openDesign = vi.fn(async () => ok);
    const stop = vi.fn(async () => ({}) as SessionResult);
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const other = session("s2", { run_started_at: "2026-10-23T09:00:00Z", status_changed_at: "2026-10-23T09:00:00Z" });
    render("/d/gx:a", [s, other], { openDesign, stop });
    await openMenu();
    const menu = await screen.findByRole("menu");
    expect(screen.getByTestId("grab-handle")).toBeTruthy();
    expect(screen.getByText("Live sessions · 2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Stop s2" }));
    await waitFor(() => expect(stop).toHaveBeenCalledWith({ id: "s2", state: stateToken(other) }));
    fireEvent.click(await newSession());
    await waitFor(() => expect(openDesign).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(menu.isConnected).toBe(false);
  } finally {
    vi.unstubAllGlobals();
  }
});

test("on a phone a press on the backdrop closes the sheet", async () => {
  phone();
  try {
    render("/d/gx:a", [session("s1")]);
    await openMenu();
    const menu = await screen.findByRole("menu");
    fireEvent.click(menu.parentElement!);
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  } finally {
    vi.unstubAllGlobals();
  }
});

test("on a phone Delete design is in the sheet while it can run", async () => {
  phone();
  try {
    render("/d/gx:a", [], {}, detailOf({ deletable: true }));
    await openMenu();
    expect(await screen.findByRole("menuitem", { name: "Delete design" })).toBeTruthy();
    expect(screen.getByTestId("grab-handle")).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
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
  await openMenu();
  fireEvent.click(await screen.findByRole("menuitem", { name: "old" }));
  fireEvent.click(await screen.findByRole("button", { name: /Continue/ }));
  await waitFor(() => expect(prompt).toHaveBeenCalled());
  expect(prompt.mock.calls[0][0]).toMatchObject({ id: "old", continue: true });
});
