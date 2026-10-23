import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicList } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import { STATS_REFETCH_DEBOUNCE_MS, type FetchStats, type Stats } from "./stats";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  tick: 0,
  streams: 0,
};

function eventConn() {
  const listeners = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  return { conn, emit: () => listeners.forEach((l) => l()) };
}

const days = ["2026-10-21", "2026-10-22", "2026-10-23"];

const statsOf = (over: Partial<Stats["throughput"]> = {}, extra: Partial<Stats> = {}): Stats => ({
  range: 30,
  days,
  throughput: {
    tickets_merged: [1, 2, 3],
    epics_merged: [0, 1, 0],
    lines_added: [10, 20, 30],
    lines_removed: [1, 2, 3],
    follow_ups_created: [1, 0, 1],
    follow_ups_finished: [0, 2, 0],
    waiting_for_design: [4, 5, 7],
    ...over,
  },
  cost: { tokens_design: [], tokens_implementation: [], spend_design: [], spend_implementation: [] },
  friction: { pause_reasons: [], pauses_by_reason: [], run_events: [], pause_endings: [] },
  ...extra,
});

const epicList = async (): Promise<EpicList> => ({
  epics: [
    { address: "gx:a", project: "gx", name: "a", kind: "loop", state: "ready", done: 0, total: 1, queued: false, queueable: true, blocked_by_epics: [], created_at: "2026-10-01T00:00:00Z" },
    { address: "web:b", project: "web", name: "b", kind: "loop", state: "ready", done: 0, total: 1, queued: false, queueable: true, blocked_by_epics: [], created_at: "2026-10-01T00:00:00Z" },
  ],
  follow_ups: [],
});

function renderStats(fetchStats: FetchStats, path = "/stats", conn: Connection = eventConn().conn) {
  const router = createMemoryRouter(appRoutes({ stats: fetchStats, epicList }), { initialEntries: [path] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

const cardText = async (title: string) => {
  const heading = await screen.findByText(title);
  let el: HTMLElement | null = heading;
  while (el && !el.textContent?.includes("Legend") && !el.querySelector("[aria-label=Legend]")) el = el.parentElement;
  return el?.textContent ?? "";
};

test("the Throughput cards follow their headline rules", async () => {
  renderStats(async () => statsOf());
  expect(await cardText("Tickets merged")).toContain("6");
  expect(await cardText("Tickets merged")).toContain("2.0/day");
  expect(await cardText("Epics merged to trunk")).toContain("1");
  const loc = await cardText("Lines of code");
  expect(loc).toContain("+60 −6");
  expect(loc).not.toContain("/day");
  const followUps = await cardText("Follow-ups");
  expect(followUps).toContain("2 created · 2 finished");
  expect(followUps).toContain("0.7/day created · 0.7/day finished");
  const waiting = await cardText("Waiting for design");
  expect(waiting).toContain("7");
  expect(waiting).not.toContain("/day");
});

test("the URL filters drive the fetch and the controls", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => statsOf());
  renderStats(fetchStats, "/stats?range=7&project=gx");
  await screen.findByText("Tickets merged");
  expect(fetchStats).toHaveBeenCalledWith({ range: 7, project: "gx" });
  expect((screen.getByLabelText("Range") as HTMLSelectElement).value).toBe("7");
  await waitFor(() => expect(within(screen.getByLabelText("Project")).getAllByRole("option").map((o) => o.textContent)).toEqual(["All projects", "gx", "web"]));
  expect((screen.getByLabelText("Project") as HTMLSelectElement).value).toBe("gx");
});

test("changing a control rewrites the URL and refetches", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => statsOf());
  const router = renderStats(fetchStats);
  await screen.findByText("Tickets merged");
  expect(fetchStats).toHaveBeenLastCalledWith({ range: 30, project: "" });
  fireEvent.change(screen.getByLabelText("Range"), { target: { value: "90" } });
  await waitFor(() => expect(fetchStats).toHaveBeenLastCalledWith({ range: 90, project: "" }));
  expect(router.state.location.search).toBe("?range=90");
});

test("a bad range falls back to 30 days", async () => {
  const fetchStats = vi.fn<FetchStats>(async () => statsOf());
  renderStats(fetchStats, "/stats?range=5");
  await screen.findByText("Tickets merged");
  expect(fetchStats).toHaveBeenCalledWith({ range: 30, project: "" });
});

test("shows the spinner first, then an error with Retry when the fetch fails", async () => {
  const fetchStats = vi.fn<FetchStats>().mockRejectedValueOnce(new Error("boom")).mockResolvedValue(statsOf());
  renderStats(fetchStats);
  expect(screen.getByRole("status", { name: "Loading stats" })).toBeTruthy();
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
  await screen.findByText("Tickets merged");
});

test("repaints from the page cache before the fetch lands", async () => {
  renderStats(async () => statsOf());
  await screen.findByText("Tickets merged");
  cleanup();
  renderStats(() => new Promise<Stats>(() => {}));
  expect(screen.queryByRole("status", { name: "Loading stats" })).toBeNull();
  expect(screen.getByText("Tickets merged")).toBeTruthy();
});

test("a server event triggers one debounced refetch", async () => {
  vi.useFakeTimers();
  const { conn, emit } = eventConn();
  const fetchStats = vi.fn<FetchStats>(async () => statsOf());
  renderStats(fetchStats, "/stats", conn);
  await act(async () => {});
  const before = fetchStats.mock.calls.length;
  act(() => {
    emit();
    emit();
  });
  await act(async () => {
    vi.advanceTimersByTime(STATS_REFETCH_DEBOUNCE_MS - 1);
  });
  expect(fetchStats).toHaveBeenCalledTimes(before);
  await act(async () => {
    vi.advanceTimersByTime(1);
  });
  expect(fetchStats).toHaveBeenCalledTimes(before + 1);
});
