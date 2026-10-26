import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetail, EpicDetailRow, EpicList, EpicListRow, MapStep, SessionInfo } from "../generated/server";
import { appRoutes } from "../routes";
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

function render(epics: EpicListRow[], sessions: SessionInfo[], path = "/d/gx:a", detail: EpicDetail = detailOf()) {
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const list: EpicList = { epics, follow_ups: [] };
  const router = createMemoryRouter(
    appRoutes({ epicList: async () => list, epic: async () => ({ kind: "ok", detail }) }),
    { initialEntries: [path] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

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
