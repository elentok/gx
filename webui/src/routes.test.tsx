import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, createConnection, TICK_MS, type Connection, type ConnectionState } from "./connection";
import { appRoutes, type RouteFetchers } from "./routes";
import type { EpicDetail, EpicDetailRow, FetchEpicDetail } from "./epic/detail";
import type { DiskStatus } from "./generated/server";
import type { Snapshot } from "./snapshot";
import type { DetailResult, FetchDetail, TicketDetail } from "./ticket/detail";
import { renderThemed } from "./ui/render";

afterEach(cleanup);

const emptySnapshot: Snapshot = {
  seq: 1,
  budget: { day: "", total: 0, soft_limit: 0, hard_limit: 0, budget_paused: false, hard_latched: false },
  health: {
    cpu: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    memory: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    disk: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
  },
  tickets: [],
  pending: [],
  sessions: [],
  epics: [],
  recently_finished: [],
};

const detailOf = (address: string): TicketDetail => ({
  address,
  title: `Title of ${address}`,
  status: "open",
  epic: "proj:work",
  map_epic: false,
  related: [],
  runs: [],
  answered: false,
  rounds: [],
  chat: [],
  actions: [],
});

const okFor: FetchDetail = async (address) => ({ kind: "ok", detail: detailOf(address) });

// A connection that never touches the network: the state is whatever the test sets.
function fakeConnection(connected: boolean, snapshot: Snapshot = emptySnapshot) {
  const state: ConnectionState = { snapshot, connected, loadedOnce: true, liveSnapshot: true, tick: 0, streams: 0 };
  const listeners = new Set<(address: string | undefined) => void>();
  return {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: (address: string | undefined) => void) => (listeners.add(l), () => listeners.delete(l)),
    refresh: () => {},
    emit: (address?: string) => listeners.forEach((l) => l(address)),
  } as unknown as Connection & { emit: (address?: string) => void };
}

function renderAt(
  path: string,
  fetchDetail: FetchDetail,
  conn: Connection = fakeConnection(true),
  fetchEpic?: FetchEpicDetail,
  extra: RouteFetchers = {},
) {
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail, epic: fetchEpic, ...extra }), { initialEntries: [path] });
  const rendered = renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { ...rendered, router };
}

const pending = <T,>() => new Promise<T>(() => {});
const title = () => document.title;

test.each([
  ["/epics", "Epics · gx"],
  ["/stats", "Stats · gx"],
  ["/stats?range=7d&project=demo", "Stats · gx"],
  ["/nonsense/here", "Epics · gx"],
])("%s sets the tab title %s", async (path, expected) => {
  renderAt(path, okFor, undefined, undefined, { stats: pending });
  await waitFor(() => expect(title()).toBe(expected));
});

test("/settings/agents renders the Agents page with the Default harness control", async () => {
  renderAt("/settings/agents", okFor, undefined, undefined, {
    agentSettings: async () => ({ default: "claude", runner: "auto", projects: [] }),
  });
  await screen.findByText("Default harness");
  expect(screen.getByRole("button", { name: "Codex" })).toBeTruthy();
});

test("the epic page titles the tab by its slug while loading", async () => {
  renderAt("/e/gx:e", okFor, undefined, () => pending());
  await waitFor(() => expect(title()).toBe("e · Epic · gx"));
});

test("the epic page titles the tab by its slug when not found", async () => {
  renderAt("/e/gx:nope", okFor, undefined, async () => ({ kind: "not-found" }));
  await screen.findByText("Epic not found");
  expect(title()).toBe("nope · Epic · gx");
});

test("the design page titles the tab before the epic list resolves and after", async () => {
  let resolve!: (l: { epics: []; follow_ups: [] }) => void;
  renderAt("/d/gx:e", okFor, undefined, undefined, { epicList: () => new Promise((r) => (resolve = r)) });
  await waitFor(() => expect(title()).toBe("e · Design · gx"));
  await act(async () => resolve({ epics: [], follow_ups: [] }));
  expect(title()).toBe("e · Design · gx");
});

test("the ticket page titles the tab by its address while the fetch is pending", async () => {
  renderAt("/t/gx:e/01", () => pending());
  await waitFor(() => expect(title()).toBe("gx:e/01 · Ticket · gx"));
});

test("the ticket page titles the tab by the loaded ticket's title", async () => {
  renderAt("/t/gx:e/01", okFor);
  await waitFor(() => expect(title()).toBe("Title of gx:e/01 · Ticket · gx"));
});

test("the ticket page titles the tab by its address when not found", async () => {
  renderAt("/t/gx:e/99", async () => ({ kind: "not-found" }));
  await screen.findByText("Ticket not found");
  expect(title()).toBe("gx:e/99 · Ticket · gx");
});

test.each([[""], ["  "]])("the ticket page titles the tab by its address for the title %j", async (blank) => {
  renderAt("/t/gx:e/01", async (address) => ({ kind: "ok", detail: { ...detailOf(address), title: blank } }));
  await waitFor(() => expect(screen.queryByRole("status", { name: "Loading ticket" })).toBeNull());
  expect(title()).toBe("gx:e/01 · Ticket · gx");
});

test("a refetch with a new ticket title changes the tab title", async () => {
  let calls = 0;
  const conn = fakeConnection(true);
  renderAt(
    "/t/gx:e/01",
    async (address) => ({ kind: "ok", detail: { ...detailOf(address), title: `Round ${++calls}` } }),
    conn,
  );
  await waitFor(() => expect(title()).toBe("Round 1 · Ticket · gx"));
  await act(async () => conn.emit("gx:e/01"));
  await waitFor(() => expect(title()).toBe("Round 2 · Ticket · gx"));
});

test("navigating between routes changes the tab title", async () => {
  const { router } = renderAt("/epics", okFor, undefined, epicFetch());
  await waitFor(() => expect(title()).toBe("Epics · gx"));
  await act(async () => {
    await router.navigate("/e/gx:e");
  });
  await waitFor(() => expect(title()).toBe("e · Epic · gx"));
});

const currentTab = () => screen.getByRole("link", { current: "page" }).textContent;

test.each(["/", "/q", "/designs", "/nonsense/here", "/epics"])("%s lands on the Epics page with the Epics tab marked and no h1", async (path) => {
  renderAt(path, okFor);
  await waitFor(() => expect(currentTab()).toBe("Epics"));
  expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
});

test.each(["/agents", "/agents/s-1"])("%s renders the Agents page with the Agents tab marked", async (path) => {
  renderAt(path, okFor);
  await waitFor(() => expect(currentTab()).toBe("Agents"));
  expect(screen.getByText("Sessions")).toBeTruthy();
});

test("the epic and ticket pages mark no tab", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch());
  await screen.findByText("No tickets yet");
  expect(screen.queryByRole("link", { current: "page" })).toBeNull();
  cleanup();
  renderAt("/t/gx:e/01", okFor);
  await screen.findByRole("heading", { name: "Title of gx:e/01" });
  expect(screen.queryByRole("link", { current: "page" })).toBeNull();
});

test("a ticket route shows a spinner, then the ticket whose address is in the hash", async () => {
  let resolve!: (r: DetailResult) => void;
  const requested: string[] = [];
  renderAt("/t/gx:e/01", (address) => {
    requested.push(address);
    return new Promise((r) => (resolve = r));
  });
  expect(await screen.findByRole("status", { name: "Loading ticket" })).toBeTruthy();
  expect(requested).toEqual(["gx:e/01"]);
  await act(async () => resolve({ kind: "ok", detail: detailOf("gx:e/01") }));
  expect(await screen.findByRole("heading", { name: "Title of gx:e/01" })).toBeTruthy();
  expect(screen.queryByRole("status", { name: "Loading ticket" })).toBeNull();
});

test("the ticket page renders under the narrow page mode", async () => {
  const queries: string[] = [];
  vi.stubGlobal("matchMedia", (q: string) => {
    queries.push(q);
    return { matches: true, addEventListener: () => {}, removeEventListener: () => {} };
  });
  try {
    renderAt("/t/gx:e/01", okFor);
    await screen.findByRole("heading", { name: "Title of gx:e/01" });
    expect(queries).toContain("(min-width: 40em)");
    expect(queries).not.toContain("(min-width: 62em)");
  } finally {
    vi.unstubAllGlobals();
  }
});

test("a 404 shows Ticket not found", async () => {
  renderAt("/t/gx:e/99", async () => ({ kind: "not-found" }));
  expect(await screen.findByText("Ticket not found")).toBeTruthy();
});

test("a failed refetch keeps the last page, and the Offline strip shows", async () => {
  let calls = 0;
  const conn = fakeConnection(false);
  renderAt(
    "/t/gx:e/01",
    async (address) => {
      if (++calls > 1) throw new Error("down");
      return { kind: "ok", detail: detailOf(address) };
    },
    conn,
  );
  expect(await screen.findByRole("heading", { name: "Title of gx:e/01" })).toBeTruthy();
  await act(async () => conn.emit("gx:e/01"));
  await waitFor(() => expect(calls).toBe(2));
  expect(screen.getByRole("heading", { name: "Title of gx:e/01" })).toBeTruthy();
  expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
});

const epicOf = (over: Partial<EpicDetail> = {}): EpicDetail => ({
  address: "gx:e",
  kind: "loop",
  summary: "",
  done: 0,
  total: 0,
  agent_time: 0,
  cost_total: 0,
  rows: [],
  queueable: false,
  queued: false,
  stats: {},
  ...over,
});

const epicFetch = (over?: Partial<EpicDetail>): FetchEpicDetail => async () => ({ kind: "ok", detail: epicOf(over) });

test("an epic route shows a spinner, then the name and summary", async () => {
  let resolve!: (r: Awaited<ReturnType<FetchEpicDetail>>) => void;
  const requested: string[] = [];
  renderAt("/e/gx:e", okFor, undefined, (address) => {
    requested.push(address);
    return new Promise((r) => (resolve = r));
  });
  expect(await screen.findByRole("status", { name: "Loading epic" })).toBeTruthy();
  expect(requested).toEqual(["gx:e"]);
  await act(async () => resolve({ kind: "ok", detail: epicOf({ summary: "Ship the thing", total: 2 }) }));
  expect(await screen.findByRole("heading", { name: "e" })).toBeTruthy();
  expect(screen.getByText("Ship the thing")).toBeTruthy();
  expect(screen.queryByText("No tickets yet")).toBeNull();
});

test("a malformed epic address shows Epic not found, not the spinner", async () => {
  vi.stubGlobal("fetch", async () => new Response("bad address", { status: 400 }));
  try {
    renderAt("/e/nocolon", okFor);
    expect(await screen.findByText("Epic not found")).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});

test("an unknown epic shows Epic not found", async () => {
  renderAt("/e/gx:nope", okFor, undefined, async () => ({ kind: "not-found" }));
  expect(await screen.findByText("Epic not found")).toBeTruthy();
});

test("an empty epic shows No tickets yet, and an empty summary renders nothing", async () => {
  const { container } = renderAt("/e/gx:e", okFor, undefined, epicFetch());
  expect(await screen.findByText("No tickets yet")).toBeTruthy();
  // The stats line and the "No tickets yet" text; an empty summary adds none.
  expect(container.querySelectorAll("p")).toHaveLength(2);
});

test("the Epics, ticket and epic pages all render inside the shared container", async () => {
  const q = renderAt("/epics", okFor);
  await screen.findByRole("link", { name: "Epics" });
  expect(q.container.querySelector("[data-page-container]")).toBeTruthy();
  cleanup();
  const t = renderAt("/t/gx:e/01", okFor);
  await screen.findByRole("heading", { name: "Title of gx:e/01" });
  expect(t.container.querySelector("[data-page-container]")).toBeTruthy();
  cleanup();
  const e = renderAt("/e/gx:e", okFor, undefined, epicFetch());
  await screen.findByText("No tickets yet");
  expect(e.container.querySelector("[data-page-container]")).toBeTruthy();
});

const rowOf = (n: string, rendered_status: string, over: Partial<EpicDetailRow> = {}): EpicDetailRow => ({
  address: `gx:e/${n}`,
  title: `Title ${n}`,
  rendered_status,
  ...over,
});

const loopRows = [
  rowOf("01", "done", { elapsed: 125, cost: 1.5 }),
  rowOf("02", "cancelled", { cost: 0.25 }),
  rowOf("03", "claimed", { elapsed: 3700 }),
  rowOf("04", "needs-repair"),
  rowOf("05", "draft"),
  rowOf("06", "blocked", { cost: 0.1 }),
];

test("an epic page draws no segmented bar", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByText("Title 03");
  expect(screen.queryByRole("progressbar", { name: "Epic progress" })).toBeNull();
});

const statsLine = () => screen.getByTestId("stats-line").textContent;

test("a loop epic shows the stat sentence with no token counts", async () => {
  renderAt(
    "/e/gx:e",
    okFor,
    undefined,
    epicFetch({ state: "running", total: 6, done: 2, wall_clock: 4000, cost_total: 1.75, agent_time: 125, rows: loopRows }),
  );
  await screen.findByTestId("stats-line");
  expect(statsLine()).toBe("Running · 1h 6m · $1.75 · 2m agent time");
  expect(screen.queryByText(/token/i)).toBeNull();
});

test("a loop epic with zero cost and agent time still shows both in the stat sentence", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "running", total: 6, done: 2, wall_clock: 4000, cost_total: 0, agent_time: 0, rows: loopRows }));
  await screen.findByTestId("stats-line");
  expect(statsLine()).toBe("Running · 1h 6m · $0.00 · 0s agent time");
});

test("a waiting epic says Not queued until it holds a queue item", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "waiting", total: 1, queueable: true }));
  await screen.findByTestId("stats-line");
  expect(statsLine()).toBe("Not queued · $0.00 · 0s agent time");
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "waiting", total: 1, queued: true }));
  await screen.findByTestId("stats-line");
  expect(statsLine()).toBe("Waiting for a slot · $0.00 · 0s agent time");
});

test("a Merge blocked epic shows its label and merge-block reason; a Merging epic its label", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "paused", total: 1, done: 1, land_block_reason: "rebase clash in a.go" }));
  await screen.findByTestId("stats-line");
  expect(statsLine()).toMatch(/^Paused · /);
  expect(screen.getByText("Can't merge: rebase clash in a.go")).toBeTruthy();
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "merging", total: 2, done: 1 }));
  await screen.findByTestId("stats-line");
  expect(statsLine()).toMatch(/^Merging · /);
});

test("live tickets show their status word, elapsed time and cost; the draft word shows on drafts", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  expect(await screen.findByText("running · 1h 1m")).toBeTruthy();
  expect(screen.getByText("needs-repair")).toBeTruthy();
  expect(screen.getByText("draft")).toBeTruthy();
  expect(screen.getByText("blocked · $0.10")).toBeTruthy();
});

test("only a claimed ticket on the epic page spins", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByText("running · 1h 1m");
  const arcs = screen.getAllByRole("img", { name: "Running" });
  expect(arcs).toHaveLength(1);
  expect(arcs[0].parentElement?.closest("[data-tone]")?.textContent).toContain("Title 03");
});

test("a map epic shows rows with title and status", async () => {
  renderAt(
    "/e/gx:m",
    okFor,
    undefined,
    epicFetch({ kind: "map", total: 2, done: 1, rows: [rowOf("01", "done"), rowOf("02", "open", { elapsed: 60 })] }),
  );
  expect(await screen.findByText("ready")).toBeTruthy();
  expect(screen.queryByText("1 of 2 done")).toBeNull();
});

test("every live, fold, Error and map row links to its ticket page", async () => {
  const hrefOf = (title: string) => screen.getByRole("link", { name: new RegExp(title) }).getAttribute("href");
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByText("running · 1h 1m");
  for (const n of ["01", "02", "03", "04", "05", "06"]) expect(hrefOf(`Title ${n}`)).toBe(`#/t/gx:e/${n}`);
  cleanup();
  renderAt(
    "/e/gx:m",
    okFor,
    undefined,
    epicFetch({ kind: "map", total: 2, done: 1, rows: [rowOf("01", "done"), rowOf("02", "open")] }),
  );
  await screen.findByText("Title 02");
  expect(hrefOf("Title 01")).toBe("#/t/gx:e/01");
  expect(hrefOf("Title 02")).toBe("#/t/gx:e/02");
});

function eventFrame(seq: number, address: string) {
  return `id: ${seq}\nevent: ticket-changed\ndata: ${JSON.stringify({ seq, type: "ticket-changed", address })}\n\n`;
}

test("an event for the page's address refetches the detail once, another epic's address does not", async () => {
  let push!: (chunk: string) => void;
  const conn = createConnection({
    fetchSnapshot: async () => emptySnapshot,
    openStream: async () =>
      (async function* () {
        for (;;) yield await new Promise<string>((r) => (push = r));
      })(),
  });
  conn.start();
  const fetched: string[] = [];
  renderAt(
    "/t/gx:e/01",
    async (address) => {
      fetched.push(address);
      return { kind: "ok", detail: detailOf(address) };
    },
    conn,
  );
  await screen.findByRole("heading", { name: "Title of gx:e/01" });
  await waitFor(() => expect(push).toBeDefined());
  expect(fetched).toHaveLength(1);

  await act(async () => push(eventFrame(2, "gx:other/02")));
  await act(async () => push(eventFrame(3, "gx:e/01")));
  await waitFor(() => expect(fetched).toHaveLength(2));
  // The foreign event came first and was processed, so a stray refetch would show as 3 by now.
  await act(async () => push(eventFrame(4, "gx:other/02")));
  expect(fetched).toEqual(["gx:e/01", "gx:e/01"]);
  conn.stop();
});

const withDetail = (over: Partial<TicketDetail>): FetchDetail => async (address) => ({
  kind: "ok",
  detail: { ...detailOf(address), ...over },
});

test("the ticket header shows the Epic button, the badge and only the stats the ticket has", async () => {
  renderAt("/t/proj:work/01", withDetail({ type: "implement" }));
  await screen.findByRole("heading", { level: 1, name: "Title of proj:work/01" });
  expect(screen.getByText("open")).toBeTruthy();
  expect(screen.getByText("implement")).toBeTruthy();
  expect(screen.getByRole("link", { name: "Epic: work" }).getAttribute("href")).toBe("#/e/proj:work");
  expect(screen.queryByRole("img", { name: "Running" })).toBeNull();
});

test("a merged ticket's stat line adds elapsed, cost and context; a map ticket has none", async () => {
  const stats = { elapsed: 600, cost: 1.5, context_window: 85000 };
  renderAt("/t/proj:work/01", withDetail({ type: "implement", status: "done", stats }));
  expect(await screen.findByText("implement · 10m · $1.50 · 85K context")).toBeTruthy();
  cleanup();
  renderAt("/t/proj:work/02", withDetail({ map_epic: true, type: "implement" }));
  await screen.findByRole("heading", { level: 1 });
  expect(screen.queryByText("implement")).toBeNull();
});

test.each(["needs-answer", "needs-repair"])("a %s ticket's badge carries the reason label", async (status) => {
  renderAt("/t/proj:work/01", withDetail({ status, pause_label: "Needs an answer" }));
  const badge = await screen.findByText("Paused · Needs an answer");
  expect(badge.getAttribute("data-tone")).toBe("paused");
});

test("a running ticket shows the arc and its elapsed time ticks", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  try {
    vi.setSystemTime(new Date("2026-01-01T10:00:00Z"));
    const conn = createConnection({ fetchSnapshot: async () => emptySnapshot, openStream: () => new Promise(() => {}) });
    conn.start();
    renderAt(
      "/t/proj:work/01",
      withDetail({ status: "claimed", type: "implement", stats: { claimed_at: "2026-01-01T09:58:00Z" } }),
      conn,
    );
    expect(await screen.findByText("implement · 2m")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Running" })).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(TICK_MS * 2);
    });
    expect(screen.getByText("implement · 4m")).toBeTruthy();
    conn.stop();
  } finally {
    vi.useRealTimers();
  }
});

test("the running last run's duration ticks", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  try {
    vi.setSystemTime(new Date("2026-01-01T10:00:00Z"));
    const conn = createConnection({ fetchSnapshot: async () => emptySnapshot, openStream: () => new Promise(() => {}) });
    conn.start();
    const runs = [{ start: "2026-01-01T09:58:00Z", outcome: "running", start_kind: "new-session" }];
    renderAt("/t/proj:work/01", withDetail({ status: "claimed", runs }), conn);
    expect(await screen.findByText(/· 2m · running · new session/)).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(TICK_MS * 2);
    });
    expect(screen.getByText(/· 4m · running · new session/)).toBeTruthy();
    conn.stop();
  } finally {
    vi.useRealTimers();
  }
});

test.each(["gx:e/99", "nonsense"])("an unknown or malformed ticket address (%s) shows Ticket not found", async (a) => {
  renderAt(`/t/${a}`, async () => ({ kind: "not-found" }));
  expect(await screen.findByText("Ticket not found")).toBeTruthy();
  expect(screen.getByText(`${a} does not exist.`)).toBeTruthy();
});

test("the ticket page refetches on an event for any ticket of its epic, not another epic's", async () => {
  let push!: (chunk: string) => void;
  const conn = createConnection({
    fetchSnapshot: async () => emptySnapshot,
    openStream: async () =>
      (async function* () {
        for (;;) yield await new Promise<string>((r) => (push = r));
      })(),
  });
  conn.start();
  let fetched = 0;
  renderAt(
    "/t/gx:e/01",
    async (address) => {
      fetched++;
      return { kind: "ok", detail: detailOf(address) };
    },
    conn,
  );
  await screen.findByRole("heading", { name: "Title of gx:e/01" });
  await waitFor(() => expect(push).toBeDefined());
  await act(async () => push(eventFrame(2, "gx:e/02")));
  await waitFor(() => expect(fetched).toBe(2));
  await act(async () => push(eventFrame(3, "gx:other/01")));
  await act(async () => push(eventFrame(4, "gx:e/03")));
  await waitFor(() => expect(fetched).toBe(3));
  conn.stop();
});

test("live forks nest under a live parent, the indent stops at 2, and a fork of a folded parent sits at the top", async () => {
  const rows = [
    rowOf("01", "open"),
    rowOf("02", "open", { parent: "gx:e/01" }),
    rowOf("03", "open", { parent: "gx:e/02" }),
    rowOf("04", "open", { parent: "gx:e/03" }),
    rowOf("05", "done"),
    rowOf("06", "open", { parent: "gx:e/05" }),
    rowOf("07", "done", { parent: "gx:e/01" }),
  ];
  const { container } = renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 7, done: 2, rows }));
  await screen.findByText("Title 01");
  const live = [...container.querySelectorAll("[data-indent]")]
    .map((el) => [el.textContent?.match(/Title \d+/)?.[0], el.getAttribute("data-indent")])
    .filter(([title]) => title !== "Title 05" && title !== "Title 07");
  expect(live).toEqual([
    ["Title 01", "0"],
    ["Title 02", "1"],
    ["Title 03", "2"],
    ["Title 04", "2"],
    ["Title 06", "0"],
  ]);
  expect(screen.getAllByText(/fork of/).map((el) => el.textContent)).toEqual(["ready · fork of 05", "fork of 01"]);
});

test("a blocked row lists its blockers, cross-epic ones as epic/NN", async () => {
  const rows = [
    rowOf("02", "blocked", { blockers: ["04", "other-epic/06"] }),
    rowOf("03", "blocked", { blockers: ["04"] }),
  ];
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("after 04 and other-epic/06")).toBeTruthy();
  expect(screen.getByText("after 04")).toBeTruthy();
});

test("an asking row shows its question and an Answer button; other rows have none", async () => {
  const rows = [
    rowOf("02", "needs-answer", { needs_answer: true, question: "Which database?" }),
    rowOf("03", "needs-repair"),
  ];
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("Which database?")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Answer" })).toHaveLength(1);
});

test("a paused row shows its reason with no Open link; a repair-class row offers Fix it with an agent", async () => {
  const rows = [
    rowOf("02", "needs-repair", { reason: "Needs repair" }),
    rowOf("03", "needs-repair", { reason: "Agent stalled" }),
  ];
  rows[1].rendered_status = "paused";
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("needs-repair · Needs repair")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Fix it with an agent" })).toHaveLength(1);
  expect(screen.getByText("paused · Agent stalled")).toBeTruthy();
  expect(screen.queryByText("Open →")).toBeNull();
});

test("a running row's elapsed time and a running epic's wall clock tick", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  try {
    vi.setSystemTime(new Date("2026-01-01T10:00:00Z"));
    const conn = createConnection({ fetchSnapshot: async () => emptySnapshot, openStream: () => new Promise(() => {}) });
    conn.start();
    const rows = [rowOf("01", "claimed", { claimed_at: "2026-01-01T09:58:00Z" })];
    renderAt("/e/gx:e", okFor, conn, epicFetch({ state: "running", total: 1, wall_clock: 600, rows }));
    expect(await screen.findByText("running · 2m")).toBeTruthy();
    expect(statsLine()).toBe("Running · 10m · $0.00 · 0s agent time");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(TICK_MS * 2);
    });
    expect(screen.getByText("running · 4m")).toBeTruthy();
    expect(statsLine()).toBe("Running · 12m · $0.00 · 0s agent time");
    conn.stop();
  } finally {
    vi.useRealTimers();
  }
});

test("the epic page refetches on an event for the epic or one of its tickets, not for another epic", async () => {
  let push!: (chunk: string) => void;
  const conn = createConnection({
    fetchSnapshot: async () => emptySnapshot,
    openStream: async () =>
      (async function* () {
        for (;;) yield await new Promise<string>((r) => (push = r));
      })(),
  });
  conn.start();
  let fetched = 0;
  renderAt("/e/gx:e", okFor, conn, async () => {
    fetched++;
    return { kind: "ok", detail: epicOf() };
  });
  await screen.findByText("No tickets yet");
  await waitFor(() => expect(push).toBeDefined());
  expect(fetched).toBe(1);

  await act(async () => push(eventFrame(2, "gx:e/03")));
  await waitFor(() => expect(fetched).toBe(2));
  await act(async () => push(eventFrame(3, "gx:e")));
  await waitFor(() => expect(fetched).toBe(3));
  // Same-prefix and foreign addresses must not match; the last event is processed after them.
  await act(async () => push(eventFrame(4, "gx:e2/01")));
  await act(async () => push(eventFrame(5, "gx:other/01")));
  await act(async () => push(eventFrame(6, "gx:e/04")));
  await waitFor(() => expect(fetched).toBe(4));
  conn.stop();
});

test("the epic page shows Queue only for a queueable epic", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ queueable: false }));
  await screen.findByRole("heading", { name: "e" });
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ queueable: true, state: "waiting" }));
  expect(await screen.findByRole("button", { name: "Queue" })).toBeTruthy();
});

test("a draft impl epic's Queue button is off", async () => {
  renderAt("/e/gx:foo-impl", okFor, undefined, epicFetch({ draft: true, queueable: false }));
  const queue = (await screen.findByRole("button", { name: "Queue" })) as HTMLButtonElement;
  expect(queue.disabled).toBe(true);
});

test("a draft non-impl epic and an open impl epic show no Queue button", async () => {
  renderAt("/e/gx:follow-ups", okFor, undefined, epicFetch({ draft: true, queueable: false }));
  await screen.findByRole("heading", { name: "follow-ups" });
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
  cleanup();
  renderAt("/e/gx:foo-impl", okFor, undefined, epicFetch({ draft: false, queueable: false }));
  await screen.findByRole("heading", { name: "foo-impl" });
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
});

// The pages also GET the epic list and the checks status; keep those out of the recorded POSTs.
function stubPosts(post: (url: string, init: RequestInit) => Promise<Response>) {
  const gets: Record<string, unknown> = {
    "/v1/epics": { epics: [], follow_ups: [] },
    "/v1/checks/status": { projects: [] },
  };
  vi.stubGlobal("fetch", (url: string, init: RequestInit) =>
    url in gets ? Promise.resolve(new Response(JSON.stringify(gets[url]))) : post(url, init),
  );
}

test("the epic page Queue posts, then refetches; a refusal shows in a toast", async () => {
  const post = vi.fn(async (_url: string, _init: RequestInit) => new Response(JSON.stringify({ queue: [] })));
  stubPosts(post);
  let fetched = 0;
  renderAt("/e/gx:e", okFor, undefined, async () => {
    fetched++;
    return { kind: "ok", detail: epicOf({ queueable: fetched === 1, state: "waiting" }) };
  });
  fireEvent.click(await screen.findByRole("button", { name: "Queue" }));
  await waitFor(() => expect(screen.queryByRole("button", { name: "Queue" })).toBeNull());
  expect(JSON.parse(post.mock.calls[0]![1].body as string)).toEqual({ address: "gx:e", state: "waiting" });
  expect(fetched).toBe(2);
  cleanup();
  post.mockImplementation(
    async () =>
      new Response(JSON.stringify({ refused: true, reason: "already-queued", message: "already in the queue" })),
  );
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ queueable: true, state: "waiting" }));
  fireEvent.click(await screen.findByRole("button", { name: "Queue" }));
  expect(await screen.findByText("already in the queue")).toBeTruthy();
  vi.unstubAllGlobals();
});

test("the epic page shows Merge only for Awaiting merge and Merge blocked", async () => {
  for (const patch of [{ state: "awaiting-merge" }, { state: "paused", land_block_reason: "rebase clash" }] as const) {
    renderAt("/e/gx:e", okFor, undefined, epicFetch(patch));
    expect(await screen.findByRole("button", { name: "Merge" })).toBeTruthy();
    cleanup();
  }
  for (const state of ["running", "merging", "finished", "waiting", undefined] as const) {
    renderAt("/e/gx:e", okFor, undefined, epicFetch({ state }));
    await screen.findByRole("heading", { name: "e" });
    expect(screen.queryByRole("button", { name: "Merge" })).toBeNull();
    cleanup();
  }
});

test("the epic page Merge posts the shown state, is disabled while pending, then refetches", async () => {
  let answer!: (r: Response) => void;
  const post = vi.fn(async (_url: string, _init: RequestInit) => new Promise<Response>((r) => (answer = r)));
  stubPosts(post);
  let fetched = 0;
  renderAt("/e/gx:e", okFor, undefined, async () => {
    fetched++;
    return { kind: "ok", detail: epicOf({ state: fetched === 1 ? "awaiting-merge" : "merging" }) };
  });
  fireEvent.click(await screen.findByRole("button", { name: "Merge" }));
  const pending = await screen.findByRole("button", { name: "Merging…" });
  expect((pending as HTMLButtonElement).disabled).toBe(true);
  expect(post.mock.calls[0]![0]).toBe("/v1/epics/merge");
  expect(JSON.parse(post.mock.calls[0]![1].body as string)).toEqual({ address: "gx:e", state: "awaiting-merge" });
  await act(async () => answer(new Response(JSON.stringify({ merge_ticket: "gx:e/09" }))));
  await waitFor(() => expect(screen.queryByRole("button", { name: /Merg/ })).toBeNull());
  expect(fetched).toBe(2);
  vi.unstubAllGlobals();
});

test("the epic page Merge shows a refusal and an HTTP error in toasts", async () => {
  const post = vi.fn(
    async () =>
      new Response(JSON.stringify({ refused: true, reason: "stale-state", message: "the epic changed" })),
  );
  stubPosts(post);
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "awaiting-merge" }));
  fireEvent.click(await screen.findByRole("button", { name: "Merge" }));
  expect(await screen.findByText("the epic changed")).toBeTruthy();
  post.mockImplementation(async () => new Response("boom: disk full", { status: 500 }));
  fireEvent.click(screen.getByRole("button", { name: "Merge" }));
  expect(await screen.findByText("boom: disk full")).toBeTruthy();
  vi.unstubAllGlobals();
});

const diskOf = (over: Partial<DiskStatus> = {}): DiskStatus => ({
  id: "d1",
  name: "Macintosh HD",
  free_bytes: 4_200_000_000,
  total_bytes: 500_000_000_000,
  projects: ["gx"],
  warned: true,
  held: false,
  read_at: "",
  cleaning: false,
  last_clean: "",
  ...over,
});

const withDisks = (...disks: DiskStatus[]) => fakeConnection(true, { ...emptySnapshot, disks });

test("no disk strip with an empty disk list", async () => {
  renderAt("/epics", okFor);
  await waitFor(() => expect(currentTab()).toBe("Epics"));
  expect(screen.queryByRole("button", { name: "Clean now" })).toBeNull();
});

test.each([
  ["/epics", undefined],
  ["/e/gx:e", epicFetch()],
  ["/t/gx:e/01", undefined],
] as const)("%s shows the disk strip", async (path, epic) => {
  renderAt(path, okFor, withDisks(diskOf()), epic);
  expect(await screen.findByRole("button", { name: "Clean now" })).toBeTruthy();
  expect(screen.getByText(/Macintosh HD/)).toBeTruthy();
});

test("two disks give two strips, a held one without a warn mark still shows, and both sit below Offline", async () => {
  const conn = fakeConnection(false, {
    ...emptySnapshot,
    disks: [diskOf(), diskOf({ id: "d2", name: "Data", warned: false, held: true })],
  });
  renderAt("/epics", okFor, conn);
  expect(await screen.findAllByRole("button", { name: "Clean now" })).toHaveLength(2);
  const strips = screen.getAllByRole("status").filter((s) => s.getAttribute("data-tone") !== null);
  expect(strips[0]!.textContent).toContain("Offline");
  expect(strips[1]!.getAttribute("data-tone")).toBe("waiting");
  expect(strips[2]!.getAttribute("data-tone")).toBe("paused");
});

test("Clean now posts the disk id", async () => {
  const post = vi.fn(async (_url: string, _init: RequestInit) => new Response(JSON.stringify({ freed_bytes: 1 })));
  stubPosts(post);
  renderAt("/epics", okFor, withDisks(diskOf()));
  fireEvent.click(await screen.findByRole("button", { name: "Clean now" }));
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post.mock.calls[0]![0]).toBe("/v1/disk/clean");
  expect(JSON.parse(post.mock.calls[0]![1].body as string)).toEqual({ disk: "d1" });
  vi.unstubAllGlobals();
});

test("a refusal shows inline and the strip stays; a failed last clean shows too", async () => {
  stubPosts(async () => new Response(JSON.stringify({ refused: true, reason: "clean-running", message: "a clean already runs" })));
  renderAt("/epics", okFor, withDisks(diskOf({ last_clean: "failed: exit 1" })));
  expect((await screen.findByRole("alert")).textContent).toBe("failed: exit 1");
  fireEvent.click(screen.getByRole("button", { name: "Clean now" }));
  await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("a clean already runs"));
  expect(screen.getByRole("button", { name: "Clean now" })).toBeTruthy();
  vi.unstubAllGlobals();
});

test("a cached snapshot with health paints the Tray before the first fetch lands", async () => {
  const meter = (state: "normal" | "high") => ({ level: 2, state, read_at: "" });
  const conn = createConnection({
    cache: { load: () => ({ ...emptySnapshot, health: { cpu: meter("high"), memory: meter("normal"), disk: meter("normal") } }), save: () => {} },
    fetchSnapshot: () => new Promise(() => {}),
    openStream: () => new Promise(() => {}),
  });
  renderAt("/epics", okFor, conn);
  expect(await screen.findByRole("button", { name: "System health: CPU high; memory normal; disk normal" })).toBeTruthy();
});

test("a cached snapshot with a disk renders the strip before the first fetch lands", async () => {
  const conn = createConnection({
    cache: { load: () => ({ ...emptySnapshot, disks: [diskOf()] }), save: () => {} },
    fetchSnapshot: () => new Promise(() => {}),
    openStream: () => new Promise(() => {}),
  });
  renderAt("/epics", okFor, conn);
  expect(await screen.findByRole("button", { name: "Clean now" })).toBeTruthy();
});
