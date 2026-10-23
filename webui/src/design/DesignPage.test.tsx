import { cleanup, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicList, EpicListRow, SessionInfo } from "../generated/server";
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
  unread: {},
  actionable: {},
  ...over,
});

function render(epics: EpicListRow[], sessions: SessionInfo[], path = "/designs") {
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const list: EpicList = { epics, follow_ups: [] };
  const router = createMemoryRouter(appRoutes({ epicList: async () => list }), { initialEntries: [path] });
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
      session("ask", { actionable: { reason: "Grilling: waiting for your answers" } }),
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
      session("ask", { actionable: { reason: "Grilling: waiting for your answers" } }),
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
  expect(screen.getByRole("link", { name: /^ns/ })).toBeTruthy();
});

test("a Ready to queue row opens the implementation epic, any other row its Design", async () => {
  render(
    [map("rdy", "ready-to-queue", { impl_address: "gx:rdy-impl" }), map("ip", "in-progress"), map("ns", "not-started")],
    [],
    "/d/gx:ip",
  );
  const href = async (n: string) => (await screen.findByRole("link", { name: new RegExp(`^${n}`) })).getAttribute("href");
  expect(await href("rdy")).toBe("#/e/gx:rdy-impl");
  expect(await href("ip")).toBe("#/d/gx:ip");
  expect(await href("ns")).toBe("#/d/gx:ns");
});
