import { fireEvent, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { createBodies, entryOptions, render, session, stubNewSession, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("a create from the New session form opens the new session", async () => {
  stubNewSession(session("q-9", "q"));
  const { router } = render([session("s-1", "q")], "/agents", true, entryOptions);
  fireEvent.click(screen.getByRole("button", { name: "New session" }));
  await screen.findByRole("combobox", { name: "Project" });
  fireEvent.click(screen.getByRole("button", { name: "Create session" }));
  await vi.waitFor(() => expect(router.state.location.pathname).toBe("/agents/q-9"));
  expect(createBodies).toHaveLength(1);
  expect(screen.queryByRole("dialog", { name: "New session" })).toBeNull();
});

test("closing the New session form stays on the page", async () => {
  stubNewSession();
  const { router } = render([session("s-1", "q")], "/agents", true, entryOptions);
  fireEvent.click(screen.getByRole("button", { name: "New session" }));
  await screen.findByRole("combobox", { name: "Project" });
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  await vi.waitFor(() => expect(screen.queryByRole("dialog", { name: "New session" })).toBeNull());
  expect(router.state.location.pathname).toBe("/agents");
  expect(createBodies).toHaveLength(0);
});

test.each([
  ["Pick a session", [session("s-1", "p")]],
  ["No sessions yet", []],
])("the %s panel's New session button opens the form", async (text, sessions) => {
  stubNewSession();
  render(sessions, "/agents", true, entryOptions);
  expect(screen.getByText(text)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "New session" }));
  expect(await screen.findByRole("dialog", { name: "New session" })).toBeTruthy();
});

test("the Agents page has no session list, switcher or Queue/Design toggles", () => {
  render([session("s-1", "p", { name: "plain" })], "/agents/s-1", true, entryOptions);
  expect(screen.queryByRole("button", { name: "New session in p" })).toBeNull();
  expect(screen.queryByRole("button", { name: /plain/ })).toBeNull();
  expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Design" })).toBeNull();
});
