import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "./connection";
import { SkillsProblemToast } from "./SkillsProblemToast";
import { renderThemed } from "./ui/render";
import { toaster } from "./ui/toaster";

afterEach(() => {
  cleanup();
  toaster.remove();
});

function setup(problem?: string) {
  let state = { snapshot: { skills_install_problem: problem }, connected: true } as unknown as ConnectionState;
  const subs = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
  } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <SkillsProblemToast />
    </ConnectionContext.Provider>,
  );
  return (next?: string) =>
    act(() => {
      state = { ...state, snapshot: { ...state.snapshot!, skills_install_problem: next } } as ConnectionState;
      subs.forEach((fn) => fn());
    });
}

test("a recorded problem shows as a toast that stays", async () => {
  setup("install refused: ~/.claude/skills/gx.md");
  expect(await screen.findByText("gx could not install its skills")).toBeTruthy();
  expect(screen.getByText("install refused: ~/.claude/skills/gx.md")).toBeTruthy();
  expect(toaster.getVisibleToasts()[0]?.duration).toBe(Infinity);
});

test("the close button dismisses it, and the same problem does not bring it back", async () => {
  const setProblem = setup("install refused");
  fireEvent.click(await screen.findByRole("button", { name: "Dismiss notification" }));
  await waitFor(() => expect(toaster.getVisibleToasts()).toHaveLength(0), { timeout: 5000 });

  setProblem("install refused");
  expect(toaster.getVisibleToasts()).toHaveLength(0);
});

test("a clean install removes the toast", async () => {
  const setProblem = setup("install refused");
  await screen.findByText("gx could not install its skills");

  setProblem(undefined);
  await waitFor(() => expect(toaster.getVisibleToasts()).toHaveLength(0));
});

test("no problem, no toast", () => {
  setup();
  expect(screen.queryByText("gx could not install its skills")).toBeNull();
});
