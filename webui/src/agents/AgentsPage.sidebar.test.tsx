import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { focusPosts, render, row, rows, session, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

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
  localStorage.setItem("gx.agents.design", "1");
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
  expect(within(row("asks")).getByRole("tooltip", { hidden: true }).textContent).toBe("★ Actionable: Review the spec");
  expect(within(row("blocked")).getByRole("img", { name: "Actionable" })).toBeTruthy();
});

test("a Queue row shows only its status mark", () => {
  render([session("q", "p", { name: "queued", queue: true, unread: { unread: true }, status: { state: "blocked" }, actionable: { reason: "Pick one" } })]);
  const queued = within(row("queued"));
  expect(queued.getByRole("img", { name: "blocked" })).toBeTruthy();
  expect(queued.queryByRole("img", { name: "Unread" })).toBeNull();
  expect(queued.queryByRole("img", { name: "Actionable" })).toBeNull();
  expect(queued.queryByRole("tooltip", { hidden: true })).toBeNull();
});

test("design sessions are hidden with a count until the toggle is on, and the choice is remembered", () => {
  const sessions = [session("1", "p", { name: "plain" }), session("2", "p", { name: "d1", design: true }), session("3", "p", { name: "d2", design: true })];
  render(sessions);
  expect(rows()).toHaveLength(1);
  expect(screen.getByText("· 2 hidden")).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox", { name: /Design sessions/ }));
  expect(rows()).toHaveLength(3);
  expect(screen.queryByText(/hidden/)).toBeNull();
  cleanup();
  render(sessions);
  expect(rows()).toHaveLength(3);
  expect((screen.getByRole("checkbox", { name: /Design sessions/ }) as HTMLInputElement).checked).toBe(true);
});

test("a selected design session keeps its row and is not counted as hidden", () => {
  render([session("1", "p", { name: "plain" }), session("2", "p", { name: "d1", design: true }), session("3", "p", { name: "d2", design: true })], "/agents/2");
  expect(rows().map((r) => r.textContent)).toEqual([expect.stringContaining("plain"), expect.stringContaining("d1")]);
  expect(screen.getByText("· 1 hidden")).toBeTruthy();
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

test("on a phone with no sessions the list says so and keeps one New session button", () => {
  render([], "/agents", true, {}, false);
  expect(screen.getByText("No sessions yet")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: /New session/ })).toHaveLength(1);
});

test("reopens the last viewed session when it is listed, else asks to pick one", () => {
  localStorage.setItem("gx.agents.last", "s-2");
  const { router } = render([session("s-1", "p"), session("s-2", "p")]);
  expect(router.state.location.pathname).toBe("/agents/s-2");
  cleanup();
  localStorage.setItem("gx.agents.last", "gone");
  const second = render([session("s-1", "p")]);
  expect(second.router.state.location.pathname).toBe("/agents");
  expect(screen.getByText("Pick a session")).toBeTruthy();
  cleanup();
  localStorage.clear();
  const third = render([session("s-1", "p")]);
  expect(third.router.state.location.pathname).toBe("/agents");
});

test("viewing a session remembers it as the last viewed", () => {
  const { router } = render([session("s-1", "p", { name: "one" }), session("s-2", "p", { name: "two" })]);
  fireEvent.click(row("two"));
  expect(localStorage.getItem("gx.agents.last")).toBe("s-2");
  expect(router.state.location.pathname).toBe("/agents/s-2");
});

test("decides nothing before the first live snapshot", () => {
  localStorage.setItem("gx.agents.last", "s-1");
  const { router, goLive } = render([session("s-1", "p")], "/agents", false);
  expect(router.state.location.pathname).toBe("/agents");
  goLive();
  expect(router.state.location.pathname).toBe("/agents/s-1");
  cleanup();
  const gone = render([session("s-1", "p")], "/agents/nope", false);
  expect(screen.queryByText(/no longer exists/)).toBeNull();
  gone.goLive();
  expect(screen.getByText("Session not found")).toBeTruthy();
});

test("an unknown label shows the not-found panel with the sidebar", () => {
  const { router } = render([session("s-1", "p", { name: "one" })], "/agents/nope");
  expect(screen.getByText("Session not found")).toBeTruthy();
  expect(screen.getByText("nope no longer exists.")).toBeTruthy();
  expect(rows()).toHaveLength(1);
  expect(router.state.location.pathname).toBe("/agents/nope");
});

test("a session removed while viewed shows the panel and the URL stays", () => {
  const { router, setSessions } = render([session("s-1", "p"), session("s-2", "p")], "/agents/s-2");
  expect(screen.queryByText("Session not found")).toBeNull();
  setSessions([session("s-1", "p")]);
  expect(screen.getByText("Session not found")).toBeTruthy();
  expect(router.state.location.pathname).toBe("/agents/s-2");
});

test("the focus report names the selected session, and none for a gone label", async () => {
  render([session("s-1", "p")], "/agents/s-1");
  await vi.waitFor(() => expect(focusPosts.at(-1)?.session).toBe("s-1"));
  cleanup();
  focusPosts.length = 0;
  render([session("s-1", "p")], "/agents/nope");
  await vi.waitFor(() => expect(focusPosts.at(-1)?.session).toBe(""));
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
