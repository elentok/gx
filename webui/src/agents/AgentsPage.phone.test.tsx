import { fireEvent, screen, within } from "@testing-library/react";
import { expect, test } from "vitest";
import { dialog, entryOptions, render, rows, session, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("phone: #/agents is the list and never redirects to the last session", () => {
  localStorage.setItem("gx.agents.last", "s-1");
  const r = render([session("s-1", "p")], "/agents", true, {}, false);
  expect(r.router.state.location.pathname).toBe("/agents");
  expect(rows()).toHaveLength(1);
});

test("phone: a session shows ‹, the title button and project · status, and ‹ returns to the list", () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, {}, false);
  expect(screen.getByRole("button", { name: /plain/ })).toBeTruthy();
  expect(screen.getByText("gx · Idle")).toBeTruthy();
  expect(rows()).toHaveLength(0);
  expect(screen.getByRole("link", { name: "All sessions" }).getAttribute("href")).toBe("#/agents");
});

test("phone: the title button opens a full-screen switcher with Close, and Close dismisses it", () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, {}, false);
  fireEvent.click(screen.getByRole("button", { name: /plain/ }));
  const d = dialog();
  expect(getComputedStyle(d.getByRole("combobox").closest("[role=dialog]")!).position).toBe("fixed");
  fireEvent.click(d.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog", { name: "Switch session" })).toBeNull();
});

test("phone: the switcher's New session row opens the form as a sheet", async () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, entryOptions, false);
  fireEvent.click(screen.getByRole("button", { name: /plain/ }));
  fireEvent.click(dialog().getByText("+ New session"));
  const form = await screen.findByRole("dialog", { name: "New session" });
  expect(within(form).getByTestId("grab-handle")).toBeTruthy();
});

test("phone: the gone panel links back to the sessions", () => {
  render([session("s-1", "p")], "/agents/nope", true, {}, false);
  expect(screen.getByRole("link", { name: "Back to sessions" }).getAttribute("href")).toBe("#/agents");
});
