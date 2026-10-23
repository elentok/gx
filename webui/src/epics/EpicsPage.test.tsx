import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicList, EpicListRow } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchEpicList } from "./list";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  tick: 0,
};
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;

const hoursAgo = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `gx:${name}`,
  project: "gx",
  name,
  kind: "loop",
  state: "ready",
  done: 0,
  total: 3,
  queued: false,
  queueable: true,
  blocked_by_epics: [],
  created_at: hoursAgo(2),
  ...over,
});

const listOf = (epics: EpicListRow[], follow_ups: EpicList["follow_ups"] = []): EpicList => ({ epics, follow_ups });

function eventConn() {
  const listeners = new Set<() => void>();
  const c = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  return { conn: c, emit: () => listeners.forEach((l) => l()) };
}

function renderEpics(fetchList: FetchEpicList, withConn: Connection = conn) {
  const router = createMemoryRouter(appRoutes({ epicList: fetchList }), { initialEntries: ["/epics"] });
  return renderThemed(
    <ConnectionContext.Provider value={withConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const loaded = async (list: EpicList) => {
  renderEpics(async () => list);
  await screen.findByRole("searchbox", { name: "Search epics" });
  await waitFor(() => expect(screen.queryByRole("status", { name: "Loading epics" })).toBeNull());
};

const names = (region: HTMLElement) => within(region).getAllByRole("link").map((l) => l.textContent);

test("sections show in order, empty ones are hidden", async () => {
  await loaded(
    listOf([
      row("fin", { state: "finished", merged_at: hoursAgo(1) }),
      row("rdy-old", { created_at: hoursAgo(5) }),
      row("rdy-new", { created_at: hoursAgo(1) }),
      row("wait-b", { kind: "map", state: "waiting-to-design" }),
      row("wait-a", { kind: "map", state: "waiting-to-design" }),
    ]),
  );
  const regions = screen.getAllByRole("region").map((r) => r.getAttribute("aria-labelledby"));
  expect(regions).toEqual(["epics-ready", "epics-waiting", "epics-finished"]);
  expect(names(screen.getByRole("region", { name: "Ready to queue" }))).toEqual(["rdy-new", "rdy-old"]);
  expect(names(screen.getByRole("region", { name: "Waiting to design" }))).toEqual(["wait-a", "wait-b"]);
  expect(screen.queryByText("Designing")).toBeNull();
});

test("a row shows project, progress and the time wording of its section", async () => {
  await loaded(
    listOf([
      row("r", { done: 1, total: 4 }),
      row("d", { kind: "map", state: "designing", last_activity_at: hoursAgo(3) }),
      row("f", { kind: "map", state: "finished", completed_at: hoursAgo(7) }),
    ]),
  );
  expect(screen.getByText("gx · 1/4 · created 2h ago")).toBeTruthy();
  expect(screen.getByText("gx · 0/3 · active 3h ago")).toBeTruthy();
  expect(screen.getByText("gx · 0/3 · finished 7h ago · map")).toBeTruthy();
});

test("a blocked ready row names its blockers as links", async () => {
  await loaded(listOf([row("x", { blocked_by_epics: ["gx:a", "blog:b"] })]));
  const region = screen.getByRole("region", { name: "Ready to queue" });
  expect(region.textContent).toContain("after a, b");
  expect(within(region).getByRole("link", { name: "b" }).getAttribute("href")).toBe("#/e/blog:b");
});

test("a Merge blocked row shows the merge-block reason", async () => {
  await loaded(listOf([row("x", { state: "merge-blocked", land_block_reason: "conflict in a.go" })]));
  fireEvent.change(screen.getByRole("searchbox", { name: "Search epics" }), { target: { value: "x" } });
  expect(screen.getByText("conflict in a.go")).toBeTruthy();
});

test("Recently finished is capped at 10 rows, newest first", async () => {
  const epics = Array.from({ length: 12 }, (_, i) => row(`e${i}`, { state: "finished", merged_at: hoursAgo(i + 1) }));
  await loaded(listOf(epics));
  const got = names(screen.getByRole("region", { name: "Recently finished" }));
  expect(got).toHaveLength(10);
  expect(got[0]).toBe("e0");
});

test.each([
  [[{ project: "gx", open: 31 }], "Follow-ups 31"],
  [
    [
      { project: "gx", open: 31 },
      { project: "blog", open: 4 },
    ],
    "Follow-ups 35 · gx 31 · blog 4",
  ],
])("the follow-ups line %#", async (followUps, text) => {
  await loaded(listOf([row("x")], followUps));
  expect(screen.getByText(/^Follow-ups/).textContent).toBe(text);
});

test("the follow-ups project names link to the follow-ups epic", async () => {
  await loaded(
    listOf([row("x")], [
      { project: "gx", open: 1 },
      { project: "blog", open: 1 },
    ]),
  );
  expect(screen.getByRole("link", { name: "blog" }).getAttribute("href")).toBe("#/e/blog:follow-ups");
});

test("no follow-ups means no line", async () => {
  await loaded(listOf([row("x")]));
  expect(screen.queryByText(/Follow-ups/)).toBeNull();
});

test("search is case-insensitive, flat, A to Z, with state badges, and clearing restores the sections", async () => {
  await loaded(
    listOf([
      row("Zeta-run", { state: "running" }),
      row("alpha-draft", { state: "draft" }),
      row("other"),
      row("map-done", { kind: "map", state: "finished", completed_at: hoursAgo(1) }),
    ]),
  );
  const box = screen.getByRole("searchbox", { name: "Search epics" });
  fireEvent.change(box, { target: { value: "A" } });
  expect(screen.queryByRole("region")).toBeNull();
  const epicLinks = screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("#/e/"));
  expect(epicLinks.map((l) => l.textContent)).toEqual(["alpha-draft", "map-done", "Zeta-run"]);
  expect(screen.getByText("Draft")).toBeTruthy();
  expect(screen.getByText("Running")).toBeTruthy();
  expect(screen.getByText("gx · 0/3 · finished 1h ago · map")).toBeTruthy();
  fireEvent.change(box, { target: { value: "" } });
  expect(screen.getAllByRole("region").length).toBeGreaterThan(0);
});

test("search rows badge the Merge blocked and Merging states", async () => {
  await loaded(listOf([row("blocked-epic", { state: "merge-blocked" }), row("merge-epic", { state: "merging" })]));
  fireEvent.change(screen.getByRole("searchbox", { name: "Search epics" }), { target: { value: "epic" } });
  expect(screen.getByText("Merge blocked")).toBeTruthy();
  expect(screen.getByText("Merging")).toBeTruthy();
});

test("a search with no match says so", async () => {
  await loaded(listOf([row("x")]));
  fireEvent.change(screen.getByRole("searchbox", { name: "Search epics" }), { target: { value: "zzz" } });
  expect(screen.getByText("No match")).toBeTruthy();
});

test("no sections shows No epics yet with the design hint", async () => {
  await loaded(listOf([row("d", { state: "draft" })]));
  expect(screen.getByText("No epics yet.")).toBeTruthy();
  expect(screen.getByText(/\/gx-design/)).toBeTruthy();
});

test("the first load shows a spinner under the search box", async () => {
  renderEpics(() => new Promise(() => {}));
  expect(await screen.findByRole("status", { name: "Loading epics" })).toBeTruthy();
  expect(screen.getByRole("searchbox", { name: "Search epics" })).toBeTruthy();
  expect(screen.queryByRole("region")).toBeNull();
});

test("a failed first fetch shows Could not load epics, and Retry fetches again", async () => {
  let calls = 0;
  renderEpics(async () => {
    if (++calls === 1) throw new Error("down");
    return listOf([row("x")]);
  });
  await screen.findByText("Could not load epics");
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));
  expect(await screen.findByRole("region", { name: "Ready to queue" })).toBeTruthy();
  expect(screen.queryByText("Could not load epics")).toBeNull();
  expect(calls).toBe(2);
});

test("a burst of events gives one refetch, and search text survives it", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const { conn: c, emit } = eventConn();
  const fetchList = vi.fn(async () => listOf([row("x")]));
  renderEpics(fetchList, c);
  const box = await screen.findByRole("searchbox", { name: "Search epics" });
  fireEvent.change(box, { target: { value: "x" } });
  await waitFor(() => expect(fetchList).toHaveBeenCalledTimes(1));
  act(() => {
    emit();
    vi.advanceTimersByTime(200);
    emit();
    vi.advanceTimersByTime(200);
    emit();
  });
  expect(fetchList).toHaveBeenCalledTimes(1);
  await act(async () => {
    vi.advanceTimersByTime(500);
  });
  expect(fetchList).toHaveBeenCalledTimes(2);
  expect((box as HTMLInputElement).value).toBe("x");
});

test("a failed refetch keeps the last list", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const { conn: c, emit } = eventConn();
  let calls = 0;
  renderEpics(async () => {
    if (++calls > 1) throw new Error("down");
    return listOf([row("keep")]);
  }, c);
  await screen.findByRole("region", { name: "Ready to queue" });
  act(() => emit());
  await act(async () => {
    vi.advanceTimersByTime(500);
  });
  expect(calls).toBe(2);
  expect(screen.getByRole("link", { name: "keep" })).toBeTruthy();
  expect(screen.queryByText("Could not load epics")).toBeNull();
});

function stateConn() {
  const subs = new Set<() => void>();
  let current = state;
  const c = {
    getState: () => current,
    subscribe: (l: () => void) => {
      subs.add(l);
      return () => subs.delete(l);
    },
    onEvent: () => () => {},
  } as unknown as Connection;
  return {
    conn: c,
    setConnected: (connected: boolean) => {
      current = { ...current, connected };
      subs.forEach((l) => l());
    },
  };
}

test("the list refetches when the connection comes back", async () => {
  const { conn: c, setConnected } = stateConn();
  let calls = 0;
  renderEpics(async () => listOf([row(++calls === 1 ? "old" : "new")]), c);
  await screen.findByRole("link", { name: "old" });
  act(() => setConnected(false));
  expect(calls).toBe(1);
  act(() => setConnected(true));
  expect(await screen.findByRole("link", { name: "new" })).toBeTruthy();
  expect(calls).toBe(2);
});

test("a failed first fetch is retried when the connection comes back", async () => {
  const { conn: c, setConnected } = stateConn();
  let calls = 0;
  renderEpics(async () => {
    if (++calls === 1) throw new Error("down");
    return listOf([row("x")]);
  }, c);
  await screen.findByText("Could not load epics");
  act(() => setConnected(false));
  act(() => setConnected(true));
  expect(await screen.findByRole("region", { name: "Ready to queue" })).toBeTruthy();
  expect(calls).toBe(2);
});

const stubQueueAdd =(reply: () => Promise<Response>) => {
  const f = vi.fn((_url: string, _init: RequestInit) => reply());
  vi.stubGlobal("fetch", f);
  return f;
};

test("Queue shows a pending label, then refetches at once and the row leaves", async () => {
  let release!: (r: Response) => void;
  const post = stubQueueAdd(() => new Promise((res) => (release = res)));
  let queued = false;
  renderEpics(async () => listOf(queued ? [] : [row("x")]));
  await screen.findByRole("region", { name: "Ready to queue" });
  fireEvent.click(screen.getByRole("button", { name: "Queue" }));
  const pending = await screen.findByRole("button", { name: "Queueing…" });
  expect((pending as HTMLButtonElement).disabled).toBe(true);
  const body = JSON.parse(post.mock.calls[0]![1]!.body as string);
  expect(body.address).toBe("gx:x");
  expect(body.state).not.toBe("");
  queued = true;
  await act(async () => release(new Response(JSON.stringify({ queue: [] }))));
  await waitFor(() => expect(screen.queryByRole("region")).toBeNull());
  vi.unstubAllGlobals();
});

test("a refused Queue shows the reason inline and keeps the row", async () => {
  stubQueueAdd(
    async () =>
      new Response(JSON.stringify({ refused: true, reason: "already-queued", message: "already in the queue" })),
  );
  renderEpics(async () => listOf([row("x")]));
  await screen.findByRole("region", { name: "Ready to queue" });
  fireEvent.click(screen.getByRole("button", { name: "Queue" }));
  expect((await screen.findByRole("alert")).textContent).toBe("already in the queue");
  expect(screen.getByRole("link", { name: "x" })).toBeTruthy();
  expect((screen.getByRole("button", { name: "Queue" }) as HTMLButtonElement).disabled).toBe(false);
  vi.unstubAllGlobals();
});
