import { fireEvent, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { dialog, entryOptions, formProject, optionNames, render, session, useAgentsPageHooks } from "./agentsTestKit";

useAgentsPageHooks();

test("+ New session opens the form prefilled with the selected session's project, and a create opens the new session", async () => {
  const trees = [
    { project: "p", root: "/dev/p", worktrees: [], tiers: {} },
    { project: "q", root: "/dev/q", worktrees: [], tiers: {} },
  ];
  const posts: unknown[] = [];
  const { router } = render([session("s-1", "q")], "/agents/s-1", true, {
    worktrees: async () => trees,
    createSession: async (body) => (posts.push(body), { session: session("q-9", "q") }),
    chat: async () => ({ kind: "not-found" }),
  });
  fireEvent.click(screen.getByRole("button", { name: "+ New session" }));
  const project = (await screen.findByRole("combobox", { name: "Project" })) as HTMLSelectElement;
  expect(project.value).toBe("q");
  fireEvent.click(screen.getByRole("button", { name: "Create session" }));
  await vi.waitFor(() => expect(router.state.location.pathname).toBe("/agents/q-9"));
  expect(posts).toEqual([expect.objectContaining({ project: "q", dir: "/dev/q" })]);
  expect(screen.queryByRole("dialog", { name: "New session" })).toBeNull();
});

test("a group's + opens the form with that project, not the selected session's", async () => {
  render([session("s-1", "p"), session("s-2", "q")], "/agents/s-1", true, entryOptions);
  fireEvent.click(screen.getByRole("button", { name: "New session in q" }));
  expect(await formProject()).toBe("q");
});

test.each([
  ["Pick a session", [session("s-1", "p")]],
  ["No sessions yet", []],
])("the %s panel's New session button opens the form", async (text, sessions) => {
  render(sessions, "/agents", true, entryOptions);
  expect(screen.getByText(text)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "New session" }));
  expect(await screen.findByRole("dialog", { name: "New session" })).toBeTruthy();
});

test("the switcher ends with + New session, reached by keys, and opens the form with the selected project", async () => {
  render([session("s-1", "p"), session("s-2", "q")], "/agents/s-2", true, entryOptions);
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  const names = optionNames();
  expect(names[names.length - 1]).toContain("+ New session");
  const input = dialog().getByRole("combobox");
  fireEvent.keyDown(input, { key: "ArrowUp" });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(screen.queryByRole("dialog", { name: "Switch session" })).toBeNull();
  expect(await formProject()).toBe("q");
});

test("the switcher's New session row stays while the filter matches nothing", () => {
  render([session("s-1", "p")], "/agents/s-1", true, entryOptions);
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  fireEvent.change(dialog().getByRole("combobox"), { target: { value: "zzz" } });
  expect(optionNames()).toEqual(["+ New session"]);
});
