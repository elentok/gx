import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { CleanupReport, CleanupWorktree, EpicList } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchCleanup, PostCleanup } from "./cleanup";

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
    { name: "old-epic", all_done: true, merged_to_main: true, has_code_review_ticket: false, code_review_done: false, action: "archive" },
    { name: "stuck-epic", all_done: true, merged_to_main: false, has_code_review_ticket: false, code_review_done: false, action: "agent" },
  ],
  worktrees: [
    wt(`${project}/merged-wt`, { path: "/w/1", action: "merged-remove", size_bytes: size }),
    wt(`${project}/merged-branch`, { action: "merged-delete" }),
    wt(`${project}/unmerged`, { path: "/w/2", action: "unmerged-remove" }),
    wt(`${project}/mystery`, { action: "agent" }),
    wt(`${project}/live`, { path: "/w/3" }),
    wt(`${project}/detached`, { branch: "", path: "/w/4", kind: "detached" }),
  ],
  attic_refs: [],
});

function renderCleanup(fetchCleanup: FetchCleanup, path = "/cleanup", conn: Connection = eventConn().conn, cleanupWrite?: PostCleanup) {
  const router = createMemoryRouter(appRoutes({ cleanup: fetchCleanup, epicList, cleanupWrite }), { initialEntries: [path] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

const names = (region: string) => within(screen.getByRole("region", { name: region })).getAllByRole("listitem").map((li) => li.querySelector("p")?.textContent);

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

const rowButton = (region: string, name: string) => within(screen.getByRole("region", { name: region })).getByRole("button", { name });

// A fetcher whose report loses the named branch or epic once a write was accepted.
function shrinking() {
  let written = false;
  const fetchCleanup: FetchCleanup = async (p) => {
    const r = reportOf(p);
    return written ? { ...r, epics: [], worktrees: [] } : r;
  };
  const post = vi.fn<PostCleanup>(async () => {
    written = true;
    return {};
  });
  return { fetchCleanup, post };
}

test.each([
  ["Merged worktrees", "Remove", "remove-worktree", { project: "gx", branch: "gx/merged-wt" }],
  ["Merged branches", "Delete branch", "delete-branch", { project: "gx", branch: "gx/merged-branch" }],
  ["Done epics", "Archive", "archive-epic", { project: "gx", epic: "old-epic" }],
])("%s: the button posts %s and the list refreshes", async (region, label, action, body) => {
  const { fetchCleanup, post } = shrinking();
  renderCleanup(fetchCleanup, "/cleanup", undefined, post);
  await screen.findByRole("region", { name: region });
  fireEvent.click(rowButton(region, label));
  await waitFor(() => expect(post).toHaveBeenCalledWith(action, expect.objectContaining(body)));
  await waitFor(() => expect(screen.queryByRole("region", { name: region })).toBeNull());
});

test("the unmerged Remove warns, Cancel posts nothing, Confirm posts", async () => {
  const { fetchCleanup, post } = shrinking();
  renderCleanup(fetchCleanup, "/cleanup", undefined, post);
  await screen.findByRole("region", { name: "Unmerged idle worktrees" });
  fireEvent.click(rowButton("Unmerged idle worktrees", "Remove"));
  expect((await screen.findByText(/Ignored files such as node_modules and build output are lost/)).textContent).toContain("The branch stays");
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByText(/Ignored files/)).toBeNull());
  expect(post).not.toHaveBeenCalled();
  fireEvent.click(rowButton("Unmerged idle worktrees", "Remove"));
  fireEvent.click(await screen.findByRole("button", { name: "Confirm remove" }));
  await waitFor(() => expect(post).toHaveBeenCalledWith("remove-unmerged-worktree", expect.objectContaining({ project: "gx", branch: "gx/unmerged" })));
  await waitFor(() => expect(screen.queryByRole("region", { name: "Unmerged idle worktrees" })).toBeNull());
});

test("a refused row stays with git's text", async () => {
  const post = vi.fn<PostCleanup>(async () => ({ refused: true, reason: "cleanup-git-refused", message: "fatal: contains modified files" }));
  renderCleanup(async (p) => reportOf(p), "/cleanup", undefined, post);
  await screen.findByRole("region", { name: "Merged worktrees" });
  fireEvent.click(rowButton("Merged worktrees", "Remove"));
  expect((await screen.findByRole("alert")).textContent).toBe("fatal: contains modified files");
  expect(names("Merged worktrees")[0]).toContain("gx/merged-wt");
});

test("dirty and live worktrees get no Remove button", async () => {
  const report: CleanupReport = {
    ...reportOf("gx"),
    epics: [],
    worktrees: [
      wt("gx/dirty-merged", { path: "/w/3", action: "merged-remove", dirty: true }),
      wt("gx/dirty", { path: "/w/4", dirty: true }),
      wt("gx/live", { path: "/w/5", active: true }),
    ],
  };
  renderCleanup(async () => report);
  await screen.findByText("gx/dirty-merged");
  expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
});

test("the agent button posts, and is absent when the group is empty", async () => {
  const post = vi.fn<PostCleanup>(async () => ({}));
  renderCleanup(async (p) => reportOf(p), "/cleanup", undefined, post);
  await screen.findByRole("region", { name: "Needs the cleanup agent" });
  fireEvent.click(rowButton("Needs the cleanup agent", "Start cleanup agent"));
  await waitFor(() => expect(post).toHaveBeenCalledWith("agent", expect.objectContaining({ project: "gx" })));
  cleanup();
  renderCleanup(async (p) => ({ ...reportOf(p), epics: [], worktrees: [] }));
  await screen.findByText("Nothing to clean up");
  expect(screen.queryByRole("button", { name: "Start cleanup agent" })).toBeNull();
});
