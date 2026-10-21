import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, createConnection, TICK_MS, type Connection, type ConnectionState } from "./connection";
import { appRoutes } from "./routes";
import type { EpicDetail, EpicDetailRow, FetchEpicDetail } from "./epic/detail";
import type { Snapshot } from "./snapshot";
import type { DetailResult, FetchDetail, TicketDetail } from "./ticket/detail";
import { renderThemed } from "./ui/render";

afterEach(cleanup);

const emptySnapshot: Snapshot = {
  seq: 1,
  budget: { day: "", total: 0, soft_limit: 0, hard_limit: 0, budget_paused: false, hard_latched: false },
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
  answered: false,
  rounds: [],
  chat: [],
  actions: [],
});

const okFor: FetchDetail = async (address) => ({ kind: "ok", detail: detailOf(address) });

// A connection that never touches the network: the state is whatever the test sets.
function fakeConnection(connected: boolean) {
  const state: ConnectionState = { snapshot: emptySnapshot, connected, loadedOnce: true, tick: 0 };
  return {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection;
}

function renderAt(
  path: string,
  fetchDetail: FetchDetail,
  conn: Connection = fakeConnection(true),
  fetchEpic?: FetchEpicDetail,
) {
  const router = createMemoryRouter(appRoutes(fetchDetail, fetchEpic), { initialEntries: [path] });
  return renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

test("the root route shows the Queue", async () => {
  renderAt("/", okFor);
  expect(await screen.findByRole("heading", { name: "Queue" })).toBeTruthy();
});

test("an unknown hash shows the Queue", async () => {
  renderAt("/nonsense/here", okFor);
  expect(await screen.findByRole("heading", { name: "Queue" })).toBeTruthy();
});

test("a ticket route shows a skeleton, then the ticket whose address is in the hash", async () => {
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

test("a 404 shows Ticket not found", async () => {
  renderAt("/t/gx:e/99", async () => ({ kind: "not-found" }));
  expect(await screen.findByText("Ticket not found")).toBeTruthy();
});

test("a failed refetch keeps the last page, and the Offline strip shows", async () => {
  let calls = 0;
  const conn = fakeConnection(false);
  let listener: ((a: string | undefined) => void) | undefined;
  (conn as unknown as { onEvent: unknown }).onEvent = (l: typeof listener) => {
    listener = l;
    return () => {};
  };
  renderAt(
    "/t/gx:e/01",
    async (address) => {
      if (++calls > 1) throw new Error("down");
      return { kind: "ok", detail: detailOf(address) };
    },
    conn,
  );
  expect(await screen.findByRole("heading", { name: "Title of gx:e/01" })).toBeTruthy();
  await act(async () => listener?.("gx:e/01"));
  await waitFor(() => expect(calls).toBe(2));
  expect(screen.getByRole("heading", { name: "Title of gx:e/01" })).toBeTruthy();
  expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
});

// jsdom's AbortSignal is rejected by the router's Request, so a click cannot navigate here.
test("the back link points at the Queue", async () => {
  renderAt("/t/gx:e/01", okFor);
  expect((await screen.findByRole("link", { name: "← Queue" })).getAttribute("href")).toBe("/");
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
  ...over,
});

const epicFetch = (over?: Partial<EpicDetail>): FetchEpicDetail => async () => ({ kind: "ok", detail: epicOf(over) });

test("an epic route shows a skeleton, then the name and summary", async () => {
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

test("a malformed epic address shows Epic not found, not the skeleton", async () => {
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
  expect(container.querySelectorAll("p")).toHaveLength(1);
});

test("the Queue, ticket and epic pages all render inside the shared container", async () => {
  const q = renderAt("/", okFor);
  await screen.findByRole("heading", { name: "Queue" });
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

test("an epic page draws the segmented bar without empty segments, cancelled counted as done", async () => {
  const { container } = renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByRole("progressbar", { name: "Epic progress" });
  const segments = [...container.querySelectorAll("[data-segment]")].map((s) => [
    s.getAttribute("data-segment"),
    (s as HTMLElement).style.width,
  ]);
  expect(segments.map(([k]) => k)).toEqual(["done", "running", "waiting", "attention"]);
  expect(segments[0][1]).toBe(`${(2 / 6) * 100}%`);
});

test("a loop epic shows the stat sentence with no token counts", async () => {
  renderAt(
    "/e/gx:e",
    okFor,
    undefined,
    epicFetch({ state: "running", total: 6, done: 2, wall_clock: 4000, cost_total: 1.75, agent_time: 125, rows: loopRows }),
  );
  expect(await screen.findByText("Running · 2 of 6 done · took 1h 6m · $1.75 · 2m of agent time")).toBeTruthy();
  expect(screen.queryByText(/token/i)).toBeNull();
});

test("a loop epic with zero cost and agent time still shows both in the stat sentence", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "running", total: 6, done: 2, wall_clock: 4000, cost_total: 0, agent_time: 0, rows: loopRows }));
  expect(await screen.findByText("Running · 2 of 6 done · took 1h 6m · $0.00 · 0s of agent time")).toBeTruthy();
});

test("the done fold counts done tickets and adds cancelled only when above zero", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  expect(await screen.findByText("✓ 1 done · 1 cancelled")).toBeTruthy();
  expect(screen.getByText("2m · $1.50")).toBeTruthy();
  expect(screen.getByText("cancelled · $0.25")).toBeTruthy();
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 1, done: 1, rows: [rowOf("01", "done")] }));
  expect(await screen.findByText("✓ 1 done")).toBeTruthy();
});

test("live tickets show their status word, elapsed time and cost; the draft word shows on drafts", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  expect(await screen.findByText("claimed · 1h 1m")).toBeTruthy();
  expect(screen.getByText("needs-repair")).toBeTruthy();
  expect(screen.getByText("draft")).toBeTruthy();
  expect(screen.getByText("blocked · $0.10")).toBeTruthy();
});

test("a map epic shows only N of M done and rows with title and status", async () => {
  renderAt(
    "/e/gx:m",
    okFor,
    undefined,
    epicFetch({ kind: "map", total: 2, done: 1, rows: [rowOf("01", "done"), rowOf("02", "open", { elapsed: 60 })] }),
  );
  expect(await screen.findByText("1 of 2 done")).toBeTruthy();
  expect(screen.getByText("open")).toBeTruthy();
});

function eventFrame(seq: number, address: string) {
  return `id: ${seq}\nevent: ticket-changed\ndata: ${JSON.stringify({ seq, type: "ticket-changed", address })}\n\n`;
}

test("an event for the page's address refetches the detail once, another address does not", async () => {
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

  await act(async () => push(eventFrame(2, "gx:e/02")));
  await act(async () => push(eventFrame(3, "gx:e/01")));
  await waitFor(() => expect(fetched).toHaveLength(2));
  // The foreign event came first and was processed, so a stray refetch would show as 3 by now.
  await act(async () => push(eventFrame(4, "gx:e/02")));
  expect(fetched).toEqual(["gx:e/01", "gx:e/01"]);
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
  const live = [...container.querySelectorAll("[data-indent]")].map((el) => [
    el.querySelector("span + span")?.textContent,
    el.getAttribute("data-indent"),
  ]);
  expect(live).toEqual([
    ["Title 01", "0"],
    ["Title 02", "1"],
    ["Title 03", "2"],
    ["Title 04", "2"],
    ["Title 06", "0"],
  ]);
  expect(screen.getAllByText(/fork of/).map((el) => el.textContent)).toEqual(["fork of 01", "open · fork of 05"]);
});

test("a blocked row lists its blockers, cross-epic ones as epic/NN", async () => {
  const rows = [
    rowOf("02", "blocked", { blockers: ["04", "other-epic/06"] }),
    rowOf("03", "blocked", { blockers: ["04"] }),
  ];
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("blocked · after 04 and other-epic/06")).toBeTruthy();
  expect(screen.getByText("blocked · after 04")).toBeTruthy();
});

test("a needs-answer row shows its question and an Answer link to the ticket page; other rows have no link", async () => {
  const rows = [
    rowOf("02", "needs-answer", { needs_answer: true, question: "Which database?" }),
    rowOf("03", "needs-repair"),
  ];
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("Which database?")).toBeTruthy();
  const links = screen.getAllByRole("link", { name: "Answer →" });
  expect(links.map((l) => l.getAttribute("href"))).toEqual(["#/t/gx:e/02"]);
});

test("a running row's elapsed time and a running epic's wall clock tick", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  try {
    vi.setSystemTime(new Date("2026-01-01T10:00:00Z"));
    const conn = createConnection({ fetchSnapshot: async () => emptySnapshot, openStream: () => new Promise(() => {}) });
    conn.start();
    const rows = [rowOf("01", "claimed", { claimed_at: "2026-01-01T09:58:00Z" })];
    renderAt("/e/gx:e", okFor, conn, epicFetch({ state: "running", total: 1, wall_clock: 600, rows }));
    expect(await screen.findByText("claimed · 2m")).toBeTruthy();
    expect(screen.getByText("Running · 0 of 1 done · took 10m · $0.00 · 0s of agent time")).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(TICK_MS * 2);
    });
    expect(screen.getByText("claimed · 4m")).toBeTruthy();
    expect(screen.getByText("Running · 0 of 1 done · took 12m · $0.00 · 0s of agent time")).toBeTruthy();
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
