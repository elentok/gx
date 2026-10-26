import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetail, EpicDetailRow, EpicList, EpicListRow, MapStep, SessionInfo } from "../generated/server";
import { appRoutes, type RouteFetchers } from "../routes";
import { renderThemed } from "../ui/render";

afterEach(cleanup);

const hoursAgo = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();

const map = (name: string, over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `gx:${name}`,
  project: "gx",
  name,
  kind: "map",
  state: "designing",
  done: 0,
  total: 0,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  ...over,
});

const session = (name: string, over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: `s-${name}`,
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: `gx:${name}/01`,
  design: true,
  status: { state: "idle" },
  status_changed_at: hoursAgo(1),
  run_started_at: hoursAgo(1),
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

const STEPS: [string, string][] = [
  ["chart", "Chart"],
  ["resolve-tickets", "Resolve tickets"],
  ["spec", "Spec"],
  ["review", "Review"],
  ["grill-findings", "Grill findings"],
  ["create-tickets", "Create tickets"],
  ["branch", "Branch"],
  ["done", "Done"],
];

// The step at `current` is current, those before are done, the rest todo.
const stepsAt = (current: number): MapStep[] =>
  STEPS.map(([step, label], i) => ({ step, label, state: i < current ? "done" : i === current ? "current" : "todo" }));

const ticketRow = (n: string, title: string, over: Partial<EpicDetailRow> = {}): EpicDetailRow => ({
  address: `gx:a/${n}`,
  title,
  rendered_status: "open",
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
  steps: stepsAt(1),
  ...over,
});

function render(
  epics: EpicListRow[],
  sessions: SessionInfo[],
  path: string | { pathname: string; state: unknown } = "/d/gx:a",
  detail: EpicDetail = detailOf(),
  fetchers: RouteFetchers = {},
) {
  let state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const listeners = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection;
  const list: EpicList = { epics, follow_ups: [] };
  const router = createMemoryRouter(
    appRoutes({ epicList: async () => list, epic: async () => ({ kind: "ok", detail }), ...fetchers }),
    { initialEntries: [path] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  const setSessions = (next: SessionInfo[]) => {
    state = { ...state, snapshot: { sessions: next } } as unknown as ConnectionState;
    act(() => listeners.forEach((fn) => fn()));
  };
  return Object.assign(router, { setSessions });
}

test("the back link counts the other Needs-you cards from the loaded list", async () => {
  render([map("a", { section: "needs-you" }), map("b", { section: "needs-you" })], []);
  expect(await screen.findByRole("link", { name: "← Epics · 1 more needs you" })).toBeTruthy();
});

test("the title row holds a back link, the Design name and the Sessions control, with no picker or state label", async () => {
  render([map("a"), map("b")], [session("a", { status: { state: "working" } })]);
  const title = await screen.findByRole("heading", { level: 1 });
  expect(within(title).getByRole("button", { name: /^a/ })).toBeTruthy();
  expect(screen.getByRole("link", { name: "← Epics" }).getAttribute("href")).toBe("/epics");
  expect(screen.getByRole("button", { name: "New session" })).toBeTruthy();
  expect(screen.queryByLabelText("Filter Designs")).toBeNull();
  expect(screen.queryByRole("link", { name: /^b/ })).toBeNull();
  expect(screen.queryByText(/designing|working/i)).toBeNull();
  expect(screen.getByRole("region", { name: "Chat" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "Progress" })).toBeTruthy();
});

test("without a session the title row offers to start one", async () => {
  render([map("a")], []);
  expect(await screen.findByRole("button", { name: "Start design session" })).toBeTruthy();
});

const failedStart = { pathname: "/d/gx:a", state: { startFailure: "agent binary missing" } };

test("a failed start's reason shows above Start design session when the map has no session", async () => {
  render([map("a")], [], failedStart);
  const button = await screen.findByRole("button", { name: "Start design session" });
  const alert = screen.getByRole("alert");
  expect(alert.textContent).toBe("agent binary missing");
  expect(alert.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test("a failed start's reason shows above New session when an exited session exists", async () => {
  render([map("a")], [session("a", { status: { state: "exited" } })], failedStart);
  const button = await screen.findByRole("button", { name: "New session" });
  const alert = screen.getByRole("alert");
  expect(alert.textContent).toBe("agent binary missing");
  expect(alert.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test("a chart-step map with an exited session shows the session header, never the New Design form", async () => {
  render([map("a")], [session("a", { status: { state: "exited" } })], "/d/gx:a", detailOf({ steps: stepsAt(0) }));
  fireEvent.click(await screen.findByRole("button", { name: "New session" }));
  expect(screen.queryByRole("form", { name: "New Design" })).toBeNull();
  expect(screen.queryByLabelText("Epic slug")).toBeNull();
});

test("a Design with no map yet and no session still offers Start design session", async () => {
  render([], []);
  expect(await screen.findByRole("button", { name: "Start design session" })).toBeTruthy();
  expect(screen.queryByRole("form", { name: "New Design" })).toBeNull();
});

const progress = async () => within(await screen.findByRole("region", { name: "Progress" }));

test("Progress shows the eight steps with their marks and the server's labels", async () => {
  render([map("a")], [], "/d/gx:a", detailOf({ steps: stepsAt(2) }));
  const p = await progress();
  await p.findByText("Spec");
  const steps = p.getAllByRole("listitem").filter((li) => li.hasAttribute("data-step"));
  expect(steps.map((li) => li.textContent)).toEqual(["✓Chart", "✓Resolve tickets", "▶Spec", "○Review", "○Grill findings", "○Create tickets", "○Branch", "○Done"]);
});

test("while Resolve tickets is current its tickets show: closed folded, current highlighted, blocked locked", async () => {
  const rows = [
    ticketRow("01", "Chart it", { rendered_status: "done" }),
    ticketRow("02", "Cancelled one", { rendered_status: "cancelled" }),
    ticketRow("03", "Chat content", { current: true }),
    ticketRow("04", "Later one", { blocked: true }),
  ];
  render([map("a")], [], "/d/gx:a", detailOf({ rows, total: 4, done: 2 }));
  const p = await progress();
  expect(await p.findByText("2 closed · 1 cancelled")).toBeTruthy();
  expect(p.queryByText("Chart it")).toBeNull();
  const current = (await p.findByText("Chat content")).closest("[data-current]");
  expect(current).toBeTruthy();
  expect(within(p.getByText("Later one").parentElement as HTMLElement).getByLabelText("Blocked")).toBeTruthy();
  expect(within(current as HTMLElement).queryByLabelText("Blocked")).toBeNull();
});

test("the tickets show only while Resolve tickets is current", async () => {
  render([map("a")], [], "/d/gx:a", detailOf({ steps: stepsAt(2), rows: [ticketRow("03", "Chat content", { current: true })] }));
  const p = await progress();
  await p.findByText("Spec");
  expect(p.queryByText("Chat content")).toBeNull();
});

test("the implementation epic link shows only at Branch and Done, once the epic exists", async () => {
  render([map("a")], [], "/d/gx:a", detailOf({ steps: stepsAt(6) }));
  await progress();
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  cleanup();
  render([map("a")], [], "/d/gx:a", detailOf({ steps: stepsAt(5), impl_address: "gx:a-impl" }));
  await progress();
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  cleanup();
  render([map("a")], [], "/d/gx:a", detailOf({ steps: stepsAt(6), impl_address: "gx:a-impl" }));
  const link = await screen.findByRole("link", { name: /Open implementation epic/ });
  expect(link.getAttribute("href")).toBe("#/e/gx:a-impl");
  expect(within(screen.getByRole("region", { name: "Chat" })).getByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
});

test("on a phone Progress is a segment bar and a line that expands on tap", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    const rows = [ticketRow("02", "Done one", { rendered_status: "done" }), ticketRow("03", "Chat content", { current: true }), ticketRow("04", "Last")];
    render([map("a")], [], "/d/gx:a", detailOf({ rows, impl_address: "gx:a-impl" }));
    const p = await progress();
    expect(await p.findByRole("progressbar", { name: "Design progress" })).toBeTruthy();
    const line = p.getByRole("button", { name: /^Resolve tickets · 03 Chat content \(2 of 3\)/ });
    expect(p.queryByText("Grill findings")).toBeNull();
    expect(p.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
    fireEvent.click(line);
    const modal = within(screen.getByRole("dialog", { name: "Design progress steps" }));
    expect(modal.getByText("Grill findings")).toBeTruthy();
    expect(modal.getByText("Chat content")).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});

test("Escape closes the phone steps modal", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    render([map("a")], [], "/d/gx:a", detailOf({ steps: stepsAt(1) }));
    const p = await progress();
    fireEvent.click(await p.findByRole("button", { name: /^Resolve tickets/ }));
    expect(screen.getByRole("dialog", { name: "Design progress steps" })).toBeTruthy();
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Design progress steps" })).toBeNull();
  } finally {
    vi.unstubAllGlobals();
  }
});

const deletable = detailOf({ deletable: true });

test("Delete design shows only for a deletable Design with no live session, following the sessions live", async () => {
  const router = render([map("a")], [], "/d/gx:a", deletable);
  expect(await screen.findByRole("button", { name: "Delete design" })).toBeTruthy();
  router.setSessions([session("a")]);
  expect(screen.queryByRole("button", { name: "Delete design" })).toBeNull();
  router.setSessions([session("a", { status: { state: "exited" } })]);
  expect(screen.getByRole("button", { name: "Delete design" })).toBeTruthy();
  cleanup();
  render([map("a")], [], "/d/gx:a", detailOf());
  await screen.findByRole("button", { name: "Start design session" });
  expect(screen.queryByRole("button", { name: "Delete design" })).toBeNull();
});

test("the detail is fetched once on mount and once more when a session starts or exits", async () => {
  const epic = vi.fn(async () => ({ kind: "ok" as const, detail: deletable }));
  const router = render([map("a")], [], "/d/gx:a", deletable, { epic });
  await screen.findByRole("button", { name: "Delete design" });
  expect(epic).toHaveBeenCalledTimes(1);
  router.setSessions([session("a")]);
  await waitFor(() => expect(epic).toHaveBeenCalledTimes(2));
  router.setSessions([session("a", { status: { state: "exited" } })]);
  await waitFor(() => expect(epic).toHaveBeenCalledTimes(3));
});

test("Keep restores the Delete design button", async () => {
  render([map("a")], [], "/d/gx:a", deletable);
  fireEvent.click(await screen.findByRole("button", { name: "Delete design" }));
  expect(screen.getByText("Delete this design?")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Keep" }));
  expect(screen.getByRole("button", { name: "Delete design" })).toBeTruthy();
});

test("confirming Delete calls the route and goes to the Epics page", async () => {
  const deleteDesign = vi.fn(async () => ({}));
  const router = render([map("a")], [], "/d/gx:a", deletable, { deleteDesign });
  fireEvent.click(await screen.findByRole("button", { name: "Delete design" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/epics"));
  expect(deleteDesign).toHaveBeenCalledWith({ epic: "gx:a" });
});

test("a refused delete shows the reason and keeps the page", async () => {
  const deleteDesign = vi.fn(async () => ({ refused: true, message: "a session is running" }));
  const router = render([map("a")], [], "/d/gx:a", deletable, { deleteDesign });
  fireEvent.click(await screen.findByRole("button", { name: "Delete design" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete" }));
  expect(await screen.findByText("a session is running")).toBeTruthy();
  expect(router.state.location.pathname).toBe("/d/gx:a");
});
