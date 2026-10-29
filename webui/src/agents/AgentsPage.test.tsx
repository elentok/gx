import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionInfo } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";

afterEach(cleanup);

const session = (id: string, project: string, over: Partial<SessionInfo> = {}): SessionInfo => ({
  id,
  project,
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

function render(sessions: SessionInfo[] | undefined, path = "/agents") {
  let state = { snapshot: sessions && { sessions }, connected: true, loadedOnce: !!sessions, tick: 0 } as unknown as ConnectionState;
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
  const router = createMemoryRouter(appRoutes(), { initialEntries: [path] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  const setSessions = (next: SessionInfo[]) => {
    state = { ...state, snapshot: { sessions: next } } as unknown as ConnectionState;
    act(() => listeners.forEach((fn) => fn()));
  };
  return { router, setSessions };
}

const rows = () => screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("#/agents/"));
const row = (name: string) => rows().find((r) => within(r).queryByText(name))!;

test("groups by project in order of first session, keeping creation order inside a group", () => {
  render([
    session("1", "beta", { name: "b1" }),
    session("2", "alpha", { name: "a1" }),
    session("3", "beta", { name: "b2" }),
    session("4", "gamma", { name: "c1" }),
    session("5", "alpha", { name: "a2" }),
  ]);
  expect(rows().map((r) => r.textContent)).toEqual(["b1", "b2", "a1", "a2", "c1"].map((n) => expect.stringContaining(n)));
  expect(screen.getAllByText(/^(beta|alpha|gamma)$/).map((h) => h.textContent)).toEqual(["beta", "alpha", "gamma"]);
});

test("shows every mark and counts non-Exited sessions as live", () => {
  render([
    session("a", "p", { name: "plain" }),
    session("b", "p", { name: "crashed", status: { state: "exited" }, exit: "crashed" }),
    session("c", "p", { name: "done", status: { state: "exited" }, exit: "clean" }),
    session("d", "p", { name: "design", design: true, ticket: "gx:m/01" }),
    session("e", "p", { name: "bg", status: { state: "working", background: true } }),
    session("f", "p", { name: "unread", unread: { unread: true } }),
    session("g", "p", { name: "asks", actionable: { reason: "Review the spec" } }),
    session("h", "p", { name: "blocked", status: { state: "blocked" } }),
  ]);
  expect(screen.getByText("6 live")).toBeTruthy();
  expect(within(row("crashed")).getByRole("img", { name: "crashed" })).toBeTruthy();
  expect(within(row("done")).getByRole("img", { name: "exited" })).toBeTruthy();
  expect(within(row("design")).getByRole("img", { name: "Design" })).toBeTruthy();
  expect(within(row("design")).getByText("gx:m/01")).toBeTruthy();
  expect(within(row("bg")).getByText("+bg")).toBeTruthy();
  expect(within(row("plain")).queryByText("+bg")).toBeNull();
  expect(within(row("unread")).getByRole("img", { name: "Unread" })).toBeTruthy();
  expect(getComputedStyle(within(row("unread")).getByText("unread")).fontWeight).toBe("700");
  expect(getComputedStyle(within(row("plain")).getByText("plain")).fontWeight).toBe("400");
  expect(within(row("plain")).queryByRole("img", { name: "Actionable" })).toBeNull();
  expect(within(row("asks")).getByRole("img", { name: "Actionable" }).getAttribute("title")).toBe("Review the spec");
  expect(within(row("blocked")).getByRole("img", { name: "Actionable" })).toBeTruthy();
});

test("a click opens the session and marks its row", () => {
  const { router } = render([session("s-1", "p", { name: "one" }), session("s-2", "p", { name: "two" })]);
  fireEvent.click(row("two"));
  expect(router.state.location.pathname).toBe("/agents/s-2");
  expect(row("two").getAttribute("aria-current")).toBe("page");
  expect(row("one").hasAttribute("aria-current")).toBe(false);
});

test("the main column says what to do, or that there is nothing yet", () => {
  render([session("s-1", "p")]);
  expect(screen.getByText("Pick a session")).toBeTruthy();
  cleanup();
  render([]);
  expect(screen.getByText("No sessions yet")).toBeTruthy();
});

test("shows the spinner until the snapshot lands, then follows it", () => {
  render(undefined);
  expect(screen.getByRole("status", { name: "Loading agents" })).toBeTruthy();
  cleanup();
  const { setSessions } = render([session("s-1", "p", { name: "one" })]);
  expect(rows()).toHaveLength(1);
  setSessions([session("s-1", "p", { name: "one" }), session("s-2", "p", { name: "two" })]);
  expect(rows()).toHaveLength(2);
});
