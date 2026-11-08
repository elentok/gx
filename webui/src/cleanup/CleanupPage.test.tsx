import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { CleanupReport, CleanupWorktree, EpicList } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchCleanup } from "./cleanup";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
};

function eventConn() {
  const listeners = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  return { conn, emit: () => listeners.forEach((l) => l()) };
}

const epicList = async (): Promise<EpicList> => ({
  epics: [
    { address: "gx:a", project: "gx", name: "a", kind: "loop", state: "ready", done: 0, total: 1, queued: false, queueable: true, blocked_by_epics: [], created_at: "2026-10-01T00:00:00Z" },
    { address: "web:b", project: "web", name: "b", kind: "loop", state: "ready", done: 0, total: 1, queued: false, queueable: true, blocked_by_epics: [], created_at: "2026-10-01T00:00:00Z" },
  ],
  follow_ups: [],
});

const wt = (branch: string, over: Partial<CleanupWorktree> = {}): CleanupWorktree => ({ branch, kind: "iteration", active: false, ...over });

const reportOf = (project: string, size?: number): CleanupReport => ({
  project,
  epics: [
    { name: "old-epic", all_done: true, merged_to_main: true, has_code_review_ticket: false, code_review_done: false },
    { name: "stuck-epic", all_done: true, merged_to_main: false, has_code_review_ticket: false, code_review_done: false, agent_group: true },
  ],
  worktrees: [
    wt(`${project}/merged-wt`, { path: "/w/1", recommendation: "delete", merged: true, delete_mode: "-d", size_bytes: size }),
    wt(`${project}/merged-branch`, { recommendation: "delete", merged: true, delete_mode: "-d" }),
    wt(`${project}/unmerged`, { path: "/w/2" }),
    wt(`${project}/mystery`, { agent_group: true }),
  ],
  attic_refs: [],
});

function renderCleanup(fetchCleanup: FetchCleanup, path = "/cleanup", conn: Connection = eventConn().conn) {
  const router = createMemoryRouter(appRoutes({ cleanup: fetchCleanup, epicList }), { initialEntries: [path] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

const names = (region: string) => within(screen.getByRole("region", { name: region })).getAllByRole("listitem").map((li) => li.textContent);

test("shows a spinner before data, then the groups, safe cases first", async () => {
  let resolve!: (r: CleanupReport) => void;
  renderCleanup(() => new Promise((r) => (resolve = r)));
  expect(await screen.findByRole("status", { name: "Loading cleanup" })).toBeTruthy();
  await waitFor(() => expect(resolve).toBeTypeOf("function"));
  await act(async () => resolve(reportOf("gx")));
  await screen.findByRole("region", { name: "Merged worktrees" });
  expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual([
    "Merged worktrees",
    "Merged branches",
    "Done epics",
    "Unmerged idle worktrees",
    "Needs the cleanup agent",
  ]);
  expect(names("Merged worktrees")).toEqual(["gx/merged-wt"]);
  expect(names("Done epics")).toEqual(["old-epic"]);
  expect(names("Unmerged idle worktrees")).toEqual(["gx/unmerged"]);
  expect(names("Needs the cleanup agent")).toEqual(["gx/mystery", "stuck-epic"]);
});

test("sizes fill in after a refetch without a reload", async () => {
  const { conn, emit } = eventConn();
  let size: number | undefined;
  renderCleanup(async (p) => reportOf(p, size), "/cleanup", conn);
  await screen.findByRole("region", { name: "Merged worktrees" });
  expect(screen.queryByTestId("cleanup-size")).toBeNull();
  size = 2048;
  vi.useFakeTimers({ shouldAdvanceTime: true });
  act(() => emit());
  await act(async () => vi.advanceTimersByTime(3000));
  vi.useRealTimers();
  expect((await screen.findByTestId("cleanup-size")).textContent).toBe("2.0 KB");
});

test("the project picker switches the read", async () => {
  const fetchCleanup = vi.fn<FetchCleanup>(async (p) => reportOf(p));
  renderCleanup(fetchCleanup);
  await screen.findByText("gx/merged-wt");
  expect(fetchCleanup).toHaveBeenCalledWith("gx");
  fireEvent.change(screen.getByLabelText("Project"), { target: { value: "web" } });
  await screen.findByText("web/merged-wt");
  await waitFor(() => expect(fetchCleanup).toHaveBeenLastCalledWith("web"));
});
