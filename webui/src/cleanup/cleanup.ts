import { apiFetch } from "../apiFetch";
import type { Connection } from "../connection";
import type { CleanupEpic, CleanupReport, CleanupWorktree, CleanupWriteRequest, CleanupWriteResult } from "../generated/server";
import { useFetched } from "../useFetched";
import { WriteError } from "../useWrite";

export type { CleanupEpic, CleanupReport, CleanupWorktree };

export type FetchCleanup = (project: string) => Promise<CleanupReport>;

export const fetchCleanup: FetchCleanup = async (project) => {
  const res = await apiFetch(`/v1/cleanup?${new URLSearchParams({ project })}`);
  if (!res.ok) throw new Error(`cleanup: HTTP ${res.status}`);
  return res.json();
};

export type CleanupAction = "remove-worktree" | "remove-unmerged-worktree" | "delete-branch" | "archive-epic" | "agent";

export type PostCleanup = (action: CleanupAction, body: CleanupWriteRequest) => Promise<CleanupWriteResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postCleanup: PostCleanup = async (action, body) => {
  const res = await apiFetch(`/v1/cleanup/${action}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  return res.json();
};

export function useCleanup(project: string, fetcher: FetchCleanup = fetchCleanup, conn?: Connection) {
  // An empty project has nothing to read yet, so the fetch stays pending and the page shows its spinner.
  const { data, failed, retry } = useFetched("cleanup", () => (project ? fetcher(project) : new Promise<CleanupReport>(() => {})), conn, project);
  return { report: project ? data : null, failed, retry };
}

export interface CleanupGroup {
  id: string;
  title: string;
  /** What the button of this group will do. */
  hint: string;
  worktrees: CleanupWorktree[];
  epics: CleanupEpic[];
}

// Safe cases first. A row lands in the first group it fits, so each shows once.
export function groupReport(report: CleanupReport): CleanupGroup[] {
  const merged = (w: CleanupWorktree) => w.recommendation === "delete" && !w.active;
  const unmergedIdle = (w: CleanupWorktree) => !w.active && !w.dirty && !!w.path && !w.recommendation;
  const groups: CleanupGroup[] = [
    { id: "merged-worktrees", title: "Merged worktrees", hint: "Remove the worktree", worktrees: report.worktrees.filter((w) => merged(w) && w.path), epics: [] },
    { id: "merged-branches", title: "Merged branches", hint: "Delete the branch", worktrees: report.worktrees.filter((w) => merged(w) && !w.path), epics: [] },
    { id: "done-epics", title: "Done epics", hint: "Archive the epic", worktrees: [], epics: report.epics.filter((e) => e.all_done && e.merged_to_main) },
    { id: "unmerged-worktrees", title: "Unmerged idle worktrees", hint: "Remove the worktree and keep the branch", worktrees: report.worktrees.filter(unmergedIdle), epics: [] },
    {
      id: "agent",
      title: "Needs the cleanup agent",
      hint: "Start a cleanup agent",
      worktrees: report.worktrees.filter((w) => w.agent_group && !unmergedIdle(w)),
      epics: report.epics.filter((e) => e.agent_group),
    },
  ];
  return groups.filter((g) => g.worktrees.length + g.epics.length > 0);
}

export function formatBytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}
