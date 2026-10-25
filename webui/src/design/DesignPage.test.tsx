import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetail, EpicDetailRow, EpicList, EpicListRow, MapStep, SessionInfo } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";

afterEach(cleanup);

const hoursAgo = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();

const map = (name: string, picker_section: EpicListRow["picker_section"], over: Partial<EpicListRow> = {}): EpicListRow => ({
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
  picker_section,
  picker_sort_at: hoursAgo(5),
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

function render(epics: EpicListRow[], sessions: SessionInfo[], path = "/designs", detail: EpicDetail = detailOf()) {
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

test("the top bar has a Designs tab", async () => {
  render([map("a", "in-progress")], []);
  expect((await screen.findByRole("link", { name: "Designs" })).getAttribute("href")).toBe("#/designs");
});

test("rows land in their four sections", async () => {
  render(
    [map("a", "in-progress"), map("b", "not-started"), map("c", "ready-to-queue"), map("d", "recently-finished"), map("old", "none")],
    [],
    "/d/gx:a",
  );
  await screen.findByText("Ready to queue");
  for (const [title, name] of [["In progress", "a"], ["Not started", "b"], ["Ready to queue", "c"], ["Recently finished", "d"]]) {
    expect(screen.getByText(title)).toBeTruthy();
    if (title === "Not started" || title === "Recently finished") fireEvent.click(screen.getByRole("button", { name: new RegExp(title) }));
    expect(screen.getAllByRole("link", { name: new RegExp(`^${name}`) })).toHaveLength(1);
  }
  expect(screen.queryByRole("link", { name: /^old/ })).toBeNull();
});

test("each mark shows", async () => {
  render(
    [
      map("live", "in-progress"),
      map("ask", "in-progress"),
      map("bad", "in-progress"),
      map("limited", "in-progress"),
      map("rdy", "ready-to-queue"),
      map("fin", "recently-finished"),
    ],
    [
      session("live", { status: { state: "working" } }),
      session("ask", { actionable: { reason: "Grilling: waiting for your answers", grilling: true } }),
      session("bad", { failure: { kind: "crashed", label: "Crashed" } }),
      session("limited", { failure: { kind: "rate-limit", label: "Rate limited" } }),
    ],
    "/d/gx:live",
  );
  const link = async (n: string) => (await screen.findByRole("link", { name: new RegExp(`^${n}`) })) as HTMLElement;
  expect(within(await link("live")).getByLabelText("Live")).toBeTruthy();
  expect(within(await link("ask")).getByLabelText("Waiting for your answer")).toBeTruthy();
  expect(within(await link("bad")).getByLabelText("Needs attention")).toBeTruthy();
  expect(within(await link("limited")).queryByLabelText("Needs attention")).toBeNull();
  expect(within(await link("rdy")).getByText("▶")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Recently finished/ }));
  expect(within(await link("fin")).getByText("✓")).toBeTruthy();
});

test("waiting and failed Designs sort first, then live ones, then newest first", async () => {
  render(
    [
      map("old", "in-progress", { picker_sort_at: hoursAgo(9) }),
      map("new", "in-progress", { picker_sort_at: hoursAgo(1) }),
      map("live", "in-progress", { picker_sort_at: hoursAgo(20) }),
      map("bad", "in-progress", { picker_sort_at: hoursAgo(30) }),
      map("ask", "in-progress", { picker_sort_at: hoursAgo(40) }),
    ],
    [
      session("live", { status: { state: "working" } }),
      session("bad", { failure: { kind: "crashed", label: "Crashed" } }),
      session("ask", { actionable: { reason: "Grilling: waiting for your answers", grilling: true } }),
    ],
    "/d/gx:new",
  );
  await screen.findByText("In progress");
  const order = screen.getAllByRole("link").map((l) => l.textContent ?? "");
  const at = (n: string) => order.findIndex((t) => t.startsWith(n));
  // Waiting and failed first (newest first among them), then live, then the rest.
  expect(at("bad")).toBeLessThan(at("ask"));
  expect(at("ask")).toBeLessThan(at("live"));
  expect(at("live")).toBeLessThan(at("new"));
  expect(at("new")).toBeLessThan(at("old"));
  expect(at("old")).toBeGreaterThanOrEqual(0);
});

test("the bare route opens the first In progress Design, never a Not started one", async () => {
  const router = render([map("ns", "not-started", { picker_sort_at: hoursAgo(1) }), map("ip", "in-progress")], []);
  await screen.findByRole("region", { name: "Chat" });
  expect(router.state.location.pathname).toBe("/d/gx:ip");
});

test("the bare route shows the picker and an empty state when nothing is in progress", async () => {
  const router = render([map("ns", "not-started")], []);
  expect(await screen.findByText("No Design in progress.")).toBeTruthy();
  expect(router.state.location.pathname).toBe("/designs");
  fireEvent.click(screen.getByRole("button", { name: /Not started/ }));
  expect(screen.getByRole("link", { name: /^ns/ })).toBeTruthy();
});

test("a Ready to queue row opens the implementation epic, any other row its Design", async () => {
  render(
    [map("rdy", "ready-to-queue", { impl_address: "gx:rdy-impl" }), map("ip", "in-progress"), map("ns", "not-started")],
    [],
    "/d/gx:ip",
  );
  fireEvent.click(await screen.findByRole("button", { name: /Not started/ }));
  const href = async (n: string) => (await screen.findByRole("link", { name: new RegExp(`^${n}`) })).getAttribute("href");
  expect(await href("rdy")).toBe("#/e/gx:rdy-impl");
  expect(await href("ip")).toBe("#/d/gx:ip");
  expect(await href("ns")).toBe("#/d/gx:ns");
});

test("Recently finished starts collapsed unless it holds the open Design", async () => {
  render([map("ip", "in-progress"), map("fin", "recently-finished")], [], "/d/gx:ip");
  await screen.findByRole("button", { name: /Recently finished/ });
  expect(screen.queryByRole("link", { name: /^fin/ })).toBeNull();
  cleanup();
  render([map("ip", "in-progress"), map("fin", "recently-finished")], [], "/d/gx:fin");
  expect(await screen.findByRole("link", { name: /^fin/ })).toBeTruthy();
});

const filterBox = async () => (await screen.findByLabelText("Filter Designs")) as HTMLInputElement;

test("Not started is collapsed with its count, opens on a filter match and closes when cleared", async () => {
  render([map("ip", "in-progress"), map("alpha", "not-started"), map("beta", "not-started"), map("gamma", "not-started")], [], "/d/gx:ip");
  await screen.findByText(/^Not started/);
  expect(screen.getByRole("button", { name: /Not started.*3/ })).toBeTruthy();
  expect(screen.queryByRole("link", { name: /^alpha/ })).toBeNull();

  fireEvent.change(await filterBox(), { target: { value: "alp" } });
  expect(screen.getByRole("button", { name: /Not started.*1/ })).toBeTruthy();
  expect(screen.getByRole("link", { name: /^alpha/ })).toBeTruthy();
  expect(screen.queryByRole("link", { name: /^beta/ })).toBeNull();
  expect(screen.queryByRole("link", { name: /^ip/ })).toBeNull();

  fireEvent.change(await filterBox(), { target: { value: "" } });
  expect(screen.queryByRole("link", { name: /^alpha/ })).toBeNull();
  expect(screen.getByRole("button", { name: /Not started.*3/ })).toBeTruthy();
});

test("the filter matches the epic slug", async () => {
  render([map("Pretty title", "in-progress", { address: "gx:the-slug" }), map("other", "in-progress")], [], "/d/gx:other");
  fireEvent.change(await filterBox(), { target: { value: "the-slug" } });
  expect(screen.getByRole("link", { name: /^Pretty title/ })).toBeTruthy();
  expect(screen.queryByRole("link", { name: /^other/ })).toBeNull();
});

test("rows show their project only with several projects", async () => {
  render([map("a", "in-progress"), map("b", "in-progress", { project: "other", address: "other:b" })], [], "/d/gx:a");
  expect(within(await screen.findByRole("link", { name: /^a/ })).getByText("gx")).toBeTruthy();
  expect(within(screen.getByRole("link", { name: /^b/ })).getByText("other")).toBeTruthy();
  cleanup();
  render([map("a", "in-progress")], [], "/d/gx:a");
  expect(within(await screen.findByRole("link", { name: /^a/ })).queryByText("gx")).toBeNull();
});

test("on a phone the picker is a dropdown with a count of other Designs that need you", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    render(
      [map("cur", "in-progress"), map("ask", "in-progress"), map("bad", "in-progress"), map("calm", "in-progress")],
      [session("ask", { actionable: { reason: "Grilling: waiting for your answers", grilling: true } }), session("bad", { failure: { kind: "crashed", label: "Crashed" } })],
      "/d/gx:cur",
    );
    const button = await screen.findByRole("button", { name: /^cur/ });
    expect(screen.getByText("? 2 more")).toBeTruthy();
    expect(screen.queryByLabelText("Filter Designs")).toBeNull();
    fireEvent.click(button);
    expect(screen.getByLabelText("Filter Designs")).toBeTruthy();
    expect(screen.getByRole("link", { name: /^calm/ })).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});

const progress = async () => within(await screen.findByRole("region", { name: "Progress" }));

test("Progress shows the eight steps with their marks and the server's labels", async () => {
  render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ steps: stepsAt(2) }));
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
  render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ rows, total: 4, done: 2 }));
  const p = await progress();
  expect(await p.findByText("2 closed · 1 cancelled")).toBeTruthy();
  expect(p.queryByText("Chart it")).toBeNull();
  const current = (await p.findByText("Chat content")).closest("[data-current]");
  expect(current).toBeTruthy();
  expect(within(p.getByText("Later one").parentElement as HTMLElement).getByLabelText("Blocked")).toBeTruthy();
  expect(within(current as HTMLElement).queryByLabelText("Blocked")).toBeNull();
});

test("the tickets show only while Resolve tickets is current", async () => {
  render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ steps: stepsAt(2), rows: [ticketRow("03", "Chat content", { current: true })] }));
  const p = await progress();
  await p.findByText("Spec");
  expect(p.queryByText("Chat content")).toBeNull();
});

test("the implementation epic link shows only at Branch and Done, once the epic exists", async () => {
  render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ steps: stepsAt(6) }));
  await progress();
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  cleanup();
  render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ steps: stepsAt(5), impl_address: "gx:a-impl" }));
  await progress();
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  cleanup();
  render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ steps: stepsAt(6), impl_address: "gx:a-impl" }));
  const link = await screen.findByRole("link", { name: /Open implementation epic/ });
  expect(link.getAttribute("href")).toBe("#/e/gx:a-impl");
  expect(within(screen.getByRole("region", { name: "Chat" })).getByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
});

test("on a phone Progress is a segment bar and a line that expands on tap", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    const rows = [ticketRow("02", "Done one", { rendered_status: "done" }), ticketRow("03", "Chat content", { current: true }), ticketRow("04", "Last")];
    render([map("a", "in-progress")], [], "/d/gx:a", detailOf({ rows, impl_address: "gx:a-impl" }));
    const p = await progress();
    expect(await p.findByRole("progressbar", { name: "Design progress" })).toBeTruthy();
    const line = p.getByRole("button", { name: /^Resolve tickets · 03 Chat content \(2 of 3\)/ });
    expect(p.queryByText("Grill findings")).toBeNull();
    expect(p.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
    fireEvent.click(line);
    expect(p.getByText("Grill findings")).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});
