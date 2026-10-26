import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
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
function fakeConnection(connected: boolean) {
  const state: ConnectionState = { snapshot: emptySnapshot, connected, loadedOnce: true, tick: 0, streams: 0 };
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
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail, epic: fetchEpic }), { initialEntries: [path] });
  return renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const currentTab = () => screen.getByRole("link", { current: "page" }).textContent;

test.each(["/", "/q", "/designs", "/nonsense/here", "/epics"])("%s lands on the Epics page with the Epics tab marked and no h1", async (path) => {
  renderAt(path, okFor);
  await waitFor(() => expect(currentTab()).toBe("Epics"));
  expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
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

test("after a direct load the ticket back link falls back to its epic", async () => {
  renderAt("/t/gx:e/01", okFor);
  expect((await screen.findByRole("link", { name: "← e" })).getAttribute("href")).toBe("/e/gx:e");
});

test("after a direct load the epic back link falls back to the Epics page", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch());
  expect((await screen.findByRole("link", { name: "← Epics" })).getAttribute("href")).toBe("/epics");
});

test("a page reached inside the app keeps its labelled back link", async () => {
  const router = createMemoryRouter(appRoutes({ ticket: okFor, epic: epicFetch() }), { initialEntries: ["/epics", "/e/gx:e"], initialIndex: 1 });
  renderThemed(
    <ConnectionContext.Provider value={fakeConnection(true)}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  expect((await screen.findByRole("link", { name: "← Epics" })).getAttribute("href")).toBe("/epics");
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
  expect(container.querySelectorAll("p")).toHaveLength(1);
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

const statRow = (container: HTMLElement, label: string) => container.querySelector(`[data-stat-row="${label}"]`)?.textContent;

test("a merged loop epic shows every Stats row below the tickets", async () => {
  const { container } = renderAt(
    "/e/gx:e",
    okFor,
    undefined,
    epicFetch({
      total: 6,
      done: 2,
      rows: loopRows,
      stats: {
        lines_added: 3412,
        lines_removed: 1208,
        total_tokens: 2_500_000,
        cost: 12.5,
        lead_time: 3 * 86400,
        friction: {
          pauses_by_reason: { deadlock: 3, conflict: 2, other: 1 },
          run_events: { stall: 4, retry: 1 },
          paused_secs: 600,
          web_share: 0.5,
        },
      },
    }),
  );
  await screen.findByRole("heading", { name: "Stats" });
  expect(statRow(container, "Lines")).toBe("Lines+3,412 −1,208");
  expect(statRow(container, "Tokens")).toBe("Tokens2.5M");
  expect(statRow(container, "Cost")).toBe("Cost$12.50");
  expect(statRow(container, "Lead time")).toBe("Lead time3dcreated → merged to trunk");
  expect(statRow(container, "Pauses")).toBe("Pauses6deadlock ×3, conflict ×2");
  expect(statRow(container, "Run events")).toBe("Run events5stall ×4, retry ×1");
  expect(statRow(container, "Paused time")).toBe("Paused time10m");
  expect(statRow(container, "Pause endings")).toBe("Pause endings50% web");
  const stats = screen.getByRole("heading", { name: "Stats" });
  expect(screen.getByText("Title 06").compareDocumentPosition(stats) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test("an unmerged loop epic shows only the lead time so far", async () => {
  const { container } = renderAt(
    "/e/gx:e",
    okFor,
    undefined,
    epicFetch({ total: 6, done: 2, rows: loopRows, stats: { lead_time: 3 * 86400, lead_time_so_far: true } }),
  );
  await screen.findByRole("heading", { name: "Stats" });
  expect(container.querySelectorAll("[data-stat-row]")).toHaveLength(1);
  expect(statRow(container, "Lead time")).toBe("Lead time3d so far");
});

test("a loop epic with no stamp has no Stats section, and cost is missing when the payload has none", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByText("Title 06");
  expect(screen.queryByRole("heading", { name: "Stats" })).toBeNull();
  cleanup();
  const { container } = renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, rows: loopRows, stats: { total_tokens: 900 } }));
  await screen.findByRole("heading", { name: "Stats" });
  expect(statRow(container, "Tokens")).toBe("Tokens900");
  expect(statRow(container, "Cost")).toBeUndefined();
});

test("a Design shows tokens, cost and lead time, or so far, only when stamped", async () => {
  const design = { kind: "map", total: 2, done: 1, rows: [rowOf("01", "done"), rowOf("02", "open")] };
  const { container } = renderAt(
    "/e/gx:e",
    okFor,
    undefined,
    epicFetch({ ...design, stats: { total_tokens: 12_000, cost: 0.5, lead_time: 7200 } }),
  );
  await screen.findByRole("heading", { name: "Stats" });
  expect(statRow(container, "Tokens")).toBe("Tokens12.0k");
  expect(statRow(container, "Cost")).toBe("Cost$0.50");
  expect(statRow(container, "Lead time")).toBe("Lead time2h");
  cleanup();
  const open = renderAt("/e/gx:e", okFor, undefined, epicFetch({ ...design, stats: { lead_time: 7200, lead_time_so_far: true } }));
  await screen.findByRole("heading", { name: "Stats" });
  expect(statRow(open.container, "Lead time")).toBe("Lead time2h so far");
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch(design));
  await screen.findByText("Title 02");
  expect(screen.queryByRole("heading", { name: "Stats" })).toBeNull();
});

test("a loop epic with zero cost and agent time still shows both in the stat sentence", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "running", total: 6, done: 2, wall_clock: 4000, cost_total: 0, agent_time: 0, rows: loopRows }));
  expect(await screen.findByText("Running · 2 of 6 done · took 1h 6m · $0.00 · 0s of agent time")).toBeTruthy();
});

test("a waiting epic says Not queued until it holds a queue item", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "waiting", total: 1, queueable: true }));
  expect(await screen.findByText("Not queued · 0 of 1 done · $0.00 · 0s of agent time")).toBeTruthy();
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "waiting", total: 1, queued: true }));
  expect(await screen.findByText("Waiting for a slot · 0 of 1 done · $0.00 · 0s of agent time")).toBeTruthy();
});

test("a Merge blocked epic shows its label and merge-block reason; a Merging epic its label", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "paused", total: 1, done: 1, land_block_reason: "rebase clash in a.go" }));
  expect(await screen.findByText(/^Paused · 1 of 1 done/)).toBeTruthy();
  expect(screen.getByText("rebase clash in a.go")).toBeTruthy();
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "merging", total: 2, done: 1 }));
  expect(await screen.findByText(/^Merging · 1 of 2 done/)).toBeTruthy();
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

test("only a claimed ticket on the epic page spins", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByText("claimed · 1h 1m");
  const arcs = screen.getAllByRole("img", { name: "Running" });
  expect(arcs).toHaveLength(1);
  expect(arcs[0].parentElement?.closest("[data-tone]")?.textContent).toContain("Title 03");
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

test("every live, fold, Error and map row links to its ticket page", async () => {
  const hrefOf = (title: string) => screen.getByRole("link", { name: title }).getAttribute("href");
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 6, done: 2, rows: loopRows }));
  await screen.findByText("claimed · 1h 1m");
  for (const n of ["01", "02", "03", "04", "05", "06"]) expect(hrefOf(`Title ${n}`)).toBe(`#/t/gx:e/${n}`);
  cleanup();
  renderAt(
    "/e/gx:m",
    okFor,
    undefined,
    epicFetch({ kind: "map", total: 2, done: 1, rows: [rowOf("01", "done"), rowOf("02", "open")] }),
  );
  await screen.findByText("1 of 2 done");
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

test("the ticket header shows the epic link, the badge and only the stats the ticket has", async () => {
  renderAt("/t/proj:work/01", withDetail({ type: "implement" }));
  await screen.findByRole("heading", { level: 1, name: "Title of proj:work/01" });
  expect(screen.getByText("open")).toBeTruthy();
  expect(screen.getByText("implement")).toBeTruthy();
  expect(screen.getByRole("link", { name: "work" }).getAttribute("href")).toBe("#/e/proj:work");
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
  expect(screen.getByRole("link", { name: /^←/ })).toBeTruthy();
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
  const live = [...container.querySelectorAll("[data-indent]")].map((el) => [
    el.querySelector("a")?.textContent,
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

test("done forks nest under a done parent in the fold, and only a fork whose parent is elsewhere says fork of", async () => {
  const rows = [
    rowOf("01", "done"),
    rowOf("02", "done", { parent: "gx:e/01" }),
    rowOf("03", "open"),
    rowOf("04", "done", { parent: "gx:e/03" }),
  ];
  const { container } = renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 4, done: 3, rows }));
  await screen.findByText("Title 01");
  const fold = [...container.querySelectorAll("[data-fold-indent]")].map((el) => [
    el.querySelector("a")?.textContent,
    el.getAttribute("data-fold-indent"),
  ]);
  expect(fold).toEqual([
    ["Title 01", "0"],
    ["Title 02", "1"],
    ["Title 04", "0"],
  ]);
  expect(screen.getAllByText(/fork of/).map((el) => el.textContent)).toEqual(["fork of 03"]);
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

test("an asking row shows its question and an Answer link to the ticket page; other rows have no link", async () => {
  const rows = [
    rowOf("02", "needs-answer", { needs_answer: true, question: "Which database?" }),
    rowOf("03", "needs-repair"),
  ];
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("Which database?")).toBeTruthy();
  const links = screen.getAllByRole("link", { name: "Answer →" });
  expect(links.map((l) => l.getAttribute("href"))).toEqual(["#/t/gx:e/02"]);
});

test("a paused row shows its reason and links to the ticket page; a repair-class row says Fix it in the TUI and has no link", async () => {
  const rows = [
    rowOf("02", "needs-repair", { reason: "Needs repair" }),
    rowOf("03", "needs-repair", { reason: "Agent stalled" }),
  ];
  rows[1].rendered_status = "paused";
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ total: 2, rows }));
  expect(await screen.findByText("needs-repair · Needs repair")).toBeTruthy();
  expect(screen.getAllByText("Fix it in the TUI")).toHaveLength(1);
  expect(screen.getByText("paused · Agent stalled")).toBeTruthy();
  const links = screen.getAllByRole("link", { name: "Open →" });
  expect(links.map((l) => l.getAttribute("href"))).toEqual(["#/t/gx:e/03"]);
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

test("the epic page shows Queue only for a queueable epic", async () => {
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ queueable: false }));
  await screen.findByRole("heading", { name: "e" });
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
  cleanup();
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ queueable: true, state: "waiting" }));
  expect(await screen.findByRole("button", { name: "Queue" })).toBeTruthy();
});

test("a draft impl epic's Queue button is off and says tickets are still publishing", async () => {
  renderAt("/e/gx:foo-impl", okFor, undefined, epicFetch({ draft: true, queueable: false }));
  const queue = (await screen.findByRole("button", { name: "Queue" })) as HTMLButtonElement;
  expect(queue.disabled).toBe(true);
  expect(screen.getByText("Still publishing tickets")).toBeTruthy();
});

test("a draft non-impl epic and an open impl epic do not show the publishing block", async () => {
  renderAt("/e/gx:follow-ups", okFor, undefined, epicFetch({ draft: true, queueable: false }));
  await screen.findByRole("heading", { name: "follow-ups" });
  expect(screen.queryByText("Still publishing tickets")).toBeNull();
  cleanup();
  renderAt("/e/gx:foo-impl", okFor, undefined, epicFetch({ draft: false, queueable: false }));
  await screen.findByRole("heading", { name: "foo-impl" });
  expect(screen.queryByText("Still publishing tickets")).toBeNull();
});

// The epic page also GETs the epic list for its jump menu; keep that out of the recorded POSTs.
function stubPosts(post: (url: string, init: RequestInit) => Promise<Response>) {
  vi.stubGlobal("fetch", (url: string, init: RequestInit) =>
    url === "/v1/epics" ? Promise.resolve(new Response(JSON.stringify({ epics: [], follow_ups: [] }))) : post(url, init),
  );
}

test("the epic page Queue posts, then refetches; a refusal shows inline", async () => {
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
  expect((await screen.findByRole("alert")).textContent).toBe("already in the queue");
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

test("the epic page Merge shows a refusal and an HTTP error inline", async () => {
  const post = vi.fn(
    async () =>
      new Response(JSON.stringify({ refused: true, reason: "stale-state", message: "the epic changed" })),
  );
  stubPosts(post);
  renderAt("/e/gx:e", okFor, undefined, epicFetch({ state: "paused", land_block_reason: "rebase clash" }));
  fireEvent.click(await screen.findByRole("button", { name: "Merge" }));
  expect((await screen.findByRole("alert")).textContent).toBe("the epic changed");
  post.mockImplementation(async () => new Response("boom", { status: 500 }));
  fireEvent.click(screen.getByRole("button", { name: "Merge" }));
  await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("Merge failed: HTTP 500"));
  vi.unstubAllGlobals();
});
