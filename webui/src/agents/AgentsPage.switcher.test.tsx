import { fireEvent, screen, within } from "@testing-library/react";
import { expect, test } from "vitest";
import type { SessionInfo } from "../generated/server";
import { dialog, openSwitcher, optionNames, render, rows, session, switcherSessions, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("the title button opens the switcher with Actionable, Unread, Other sections in order", () => {
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  expect(dialog().getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Actionable", "Unread", "Other"]);
  const names = optionNames();
  expect(names[0]).toContain("needs you");
  expect(names[0]).toContain("beta · Idle · gx:m/07 · Pick one");
  expect(names[1]).toContain("unread one");
  expect(names.slice(2).join()).toContain("plain");
});

test("the title button and Cmd-K open the switcher on the filter", () => {
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  expect(document.activeElement).toBe(dialog().getByRole("combobox"));
  fireEvent.keyDown(window, { key: "Escape" });
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(document.activeElement).toBe(dialog().getByRole("combobox"));
});

test("the switcher title shows no ⌘K hint", () => {
  render(switcherSessions(), "/agents/s-1");
  expect(screen.getByRole("heading", { level: 1 }).textContent).not.toContain("⌘K");
});

test("Cmd-K and Ctrl-K open the switcher", () => {
  render(switcherSessions(), "/agents/s-1");
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(screen.getByRole("dialog", { name: "Switch session" })).toBeTruthy();
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(screen.getByRole("dialog", { name: "Switch session" })).toBeTruthy();
});

const showGroups = () => {
  localStorage.setItem("gx.agents.design", "1");
  localStorage.setItem("gx.agents.queue", "1");
};

test("Cmd-K opens the switcher before a session is picked", () => {
  showGroups();
  render(switcherSessions(), "/agents");
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(dialog().getAllByRole("option")).toHaveLength(5);
});

test("the filter matches the ticket and the dir", () => {
  showGroups();
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  const input = dialog().getByRole("combobox");
  fireEvent.change(input, { target: { value: "m/07" } });
  expect(optionNames()).toHaveLength(2);
  expect(optionNames()[0]).toContain("needs you");
  fireEvent.change(input, { target: { value: "/work/design" } });
  expect(optionNames()[0]).toContain("the design");
});

test("the switcher follows the Queue/Design toggles", () => {
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  expect(optionNames().join()).not.toContain("the design");
});

test("a design session is listed once its toggle is on, and the current one is tagged", () => {
  showGroups();
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  const design = dialog().getAllByRole("option").find((o) => o.textContent?.includes("the design"))!;
  expect(within(design).getByRole("img", { name: "Design" })).toBeTruthy();
  expect(dialog().getAllByRole("option").filter((o) => o.getAttribute("aria-current") === "true")).toHaveLength(1);
  expect(dialog().getByText("current")).toBeTruthy();
});

test("Design and Queue rows carry no Unread or Actionable mark and sort under Other", () => {
  showGroups();
  const asks = { unread: { unread: true }, actionable: { reason: "Pick one" } };
  render([session("s-1", "alpha", { name: "plain" }), session("d", "alpha", { name: "dz", design: true, ...asks }), session("q", "alpha", { name: "qz", queue: true, ...asks })], "/agents/s-1");
  openSwitcher();
  for (const name of ["dz", "qz"]) {
    const option = dialog().getAllByRole("option").find((o) => o.textContent?.includes(name))!;
    expect(within(option).queryByRole("img", { name: "Unread" })).toBeNull();
    expect(within(option).queryByRole("img", { name: "Actionable" })).toBeNull();
  }
  expect(dialog().queryByText("Actionable")).toBeNull();
});

test("switcher rows use the sidebar marks: purple design diamond and unread dot, ? for blocked", () => {
  showGroups();
  const blocked = session("s-5", "beta", { name: "stuck" });
  blocked.status = { state: "blocked" } as SessionInfo["status"];
  render([...switcherSessions(), blocked], "/agents/s-1");
  openSwitcher();
  const option = (text: string) => dialog().getAllByRole("option").find((o) => o.textContent?.includes(text))!;
  expect(within(option("the design")).getByRole("img", { name: "Design" }).textContent).toBe("◆");
  expect(within(option("unread one")).getByRole("img", { name: "Unread" }).className).toBe(
    within(rows().find((r) => r.textContent?.includes("unread one"))!).getByRole("img", { name: "Unread" }).className,
  );
  expect(within(option("stuck")).getByRole("img", { name: "blocked" }).textContent).toBe("?");
});

test("Enter on a row navigates to it, and Escape closes", () => {
  const { router } = render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  fireEvent.keyDown(dialog().getByRole("combobox"), { key: "Enter" });
  expect(router.state.location.pathname).toBe("/agents/s-3");
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /needs you/ }));
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
});
