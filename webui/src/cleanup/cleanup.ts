import { apiFetch } from "../apiFetch"
import type { Connection } from "../connection"
import type {
  CleanupEpic,
  CleanupReport,
  CleanupWorktree,
  CleanupWriteRequest,
  CleanupWriteResult,
} from "../generated/server"
import { useFetched } from "../useFetched"
import { WriteError } from "../useWrite"

export type { CleanupEpic, CleanupReport, CleanupWorktree }

export type FetchCleanup = (project: string) => Promise<CleanupReport>

export const fetchCleanup: FetchCleanup = async (project) => {
  const res = await apiFetch(`/v1/cleanup?${new URLSearchParams({ project })}`)
  if (!res.ok) throw new Error(`cleanup: HTTP ${res.status}`)
  return res.json()
}

export type CleanupAction =
  | "remove-worktree"
  | "remove-unmerged-worktree"
  | "delete-branch"
  | "archive-epic"
  | "agent"

export type PostCleanup = (
  action: CleanupAction,
  body: CleanupWriteRequest,
) => Promise<CleanupWriteResult>

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postCleanup: PostCleanup = async (action, body) => {
  const res = await apiFetch(`/v1/cleanup/${action}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
  return res.json()
}

export function useCleanup(
  project: string,
  fetcher: FetchCleanup = fetchCleanup,
  conn?: Connection,
) {
  // An empty project has nothing to read yet, so the fetch stays pending and the page shows its spinner.
  const { data, failed, retry } = useFetched(
    "cleanup",
    () => (project ? fetcher(project) : new Promise<CleanupReport>(() => {})),
    conn,
    project,
  )
  return { report: project ? data : null, failed, retry }
}

export interface CleanupGroup {
  id: string
  title: string
  /** What the button of this group will do. */
  hint: string
  worktrees: CleanupWorktree[]
  epics: CleanupEpic[]
}

// Safe cases first. The server decides each row's action; this only buckets by it.
export function groupReport(report: CleanupReport): CleanupGroup[] {
  const wts = (action: string) => report.worktrees.filter((w) => w.action === action)
  const epics = (action: string) => report.epics.filter((e) => e.action === action)
  const groups: CleanupGroup[] = [
    {
      id: "merged-worktrees",
      title: "Merged worktrees",
      hint: "Remove the worktree",
      worktrees: wts("merged-remove"),
      epics: [],
    },
    {
      id: "merged-branches",
      title: "Merged branches",
      hint: "Delete the branch",
      worktrees: wts("merged-delete"),
      epics: [],
    },
    {
      id: "done-epics",
      title: "Done epics",
      hint: "Archive the epic",
      worktrees: [],
      epics: epics("archive"),
    },
    {
      id: "unmerged-worktrees",
      title: "Unmerged idle worktrees",
      hint: "Remove the worktree and keep the branch",
      worktrees: wts("unmerged-remove"),
      epics: [],
    },
    {
      id: "agent",
      title: "Needs the cleanup agent",
      hint: "Start a cleanup agent",
      worktrees: wts("agent"),
      epics: epics("agent"),
    },
  ]
  return groups.filter((g) => g.worktrees.length + g.epics.length > 0)
}
