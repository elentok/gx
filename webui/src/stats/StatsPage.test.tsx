import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicList, StatsFriction } from "../generated/server";
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
  grilling: { words: [], questions: [] },
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
  expect(waiting).toContain("maps open now");
  expect(waiting).not.toContain("/day");
});

const cardEl = async (title: string) => {
  let el: HTMLElement | null = await screen.findByText(title);
  while (el && !el.querySelector("svg")) el = el.parentElement;
  return el as HTMLElement;
};

const amount = (value: number, no_data = false) => ({ value, no_data });

test("the Cost cards hatch a no-data day and say partial", async () => {
  const cost = {
    tokens_design: [amount(100), amount(0, true), amount(300)],
    tokens_implementation: [amount(1000), amount(0, true), amount(2000)],
    spend_design: [amount(1), amount(0, true), amount(2)],
    spend_implementation: [amount(3), amount(0, true), amount(4.5)],
  };
  renderStats(async () => statsOf({}, { cost }));
  const tokens = await cardText("Tokens");
  expect(tokens).toContain("3,400");
  expect(tokens).toContain("partial");
  expect((await cardEl("Tokens")).querySelectorAll("[data-nodata]")).toHaveLength(1);
  const spend = await cardText("Spend");
  expect(spend).toContain("$10.50");
  expect(spend).toContain("Design");
  expect(spend).toContain("Implementation");
});

test("Cost cards with data on every day are not partial", async () => {
  const full = [amount(1), amount(1), amount(1)];
  renderStats(async () => statsOf({}, { cost: { tokens_design: full, tokens_implementation: full, spend_design: full, spend_implementation: full } }));
  expect(await cardText("Tokens")).not.toContain("partial");
});

test("the Grilling section sits between Cost and effort and Friction", async () => {
  renderStats(async () => statsOf());
  await screen.findByText("Words read");
  // The toaster's own container is a region too; only the sections name themselves.
  const names = screen
    .getAllByRole("region")
    .map((r) => r.getAttribute("aria-labelledby"))
    .filter((n) => n !== null);
  expect(names).toEqual(["stats-throughput", "stats-cost", "stats-grilling", "stats-friction"]);
  expect(screen.getByText("Questions asked")).toBeTruthy();
});

test("the Grilling cards skip no-data days in the total and average and hatch them", async () => {
  const grilling = {
    words: [amount(1000), amount(0, true), amount(2500)],
    questions: [amount(4), amount(0, true), amount(6)],
  };
  renderStats(async () => statsOf({}, { grilling }));
  const words = await cardText("Words read");
  expect(words).toContain("3,500");
  expect(words).toContain("1,750/day");
  expect(words).toContain("partial");
  expect((await cardEl("Words read")).querySelectorAll("[data-nodata]")).toHaveLength(1);
  const questions = await cardText("Questions asked");
  expect(questions).toContain("10");
  expect(questions).toContain("5/day");
  expect((await cardEl("Questions asked")).querySelectorAll("[data-nodata]")).toHaveLength(1);
});

test("the Pauses tooltip lists a reason folded into other", async () => {
  const friction: StatsFriction = {
    pause_reasons: ["question", "other"],
    pauses_by_reason: [{ question: 2, stalled: 1 }, {}, { question: 1 }],
    run_events: [{}, {}, {}],
    pause_endings: [{}, {}, {}],
  };
  renderStats(async () => statsOf({}, { friction }));
  const card = await cardEl("Pauses");
  expect(card.textContent).toContain("other");
  fireEvent.mouseEnter(card.querySelector("[data-hit='0']") as Element);
  const tip = within(screen.getByRole("tooltip"));
  expect(tip.getByText("question: 2")).toBeTruthy();
  expect(tip.getByText("other: 1")).toBeTruthy();
  expect(tip.getByText("stalled: 1")).toBeTruthy();
  expect(card.querySelectorAll("[data-bar][data-series='stalled']")).toHaveLength(0);
});

test("Pause endings headline is the web share with the median on the small line", async () => {
  const friction: StatsFriction = {
    pause_reasons: [],
    pauses_by_reason: [{}, {}, {}],
    run_events: [{}, {}, {}],
    pause_endings: [{ web: 5, tui: 2, gx: 4 }, {}, { web: 0, cli: 1 }],
    web_share: 0.625,
    median_resume_secs: 270,
  };
  renderStats(async () => statsOf({}, { friction }));
  const ending = await cardText("Pause endings");
  expect(ending).toContain("63% web");
  expect(ending).toContain("median 5m to resume");
  expect(ending).not.toContain("partial");
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
