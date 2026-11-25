// The Diff page's address: every part of the view lives in the query, so a reload or a pasted link
// shows the same view. This module owns the query names; nothing else spells them.

export const WORKTREE_KINDS = ["chat", "design", "ticket"] as const

/** A worktree diff's target. The id of a ticket holds colons, so only the first one splits. */
export interface WorktreeTarget {
  kind: (typeof WORKTREE_KINDS)[number]
  id: string
}

/**
 * The narrowing to the changes between HEAD and the snapshot, spelled as the one entry of `commits`.
 * It cannot be a SHA: each refetch builds a new snapshot.
 */
export const UNCOMMITTED = "uncommitted"

export const worktreeValue = (t: WorktreeTarget) => `${t.kind}:${t.id}`

function parseWorktree(v: string): WorktreeTarget | undefined {
  const i = v.indexOf(":")
  const kind = v.slice(0, i)
  const id = v.slice(i + 1)
  const known = WORKTREE_KINDS.find((k) => k === kind)
  return known && id ? { kind: known, id } : undefined
}

export interface DiffUrl {
  project: string
  /** The base ref, as typed. Empty in worktree mode. */
  first: string
  /** The tip ref, as typed. Empty in worktree mode. */
  second: string
  /** Worktree mode: the page diffs this target's worktree; the base is never in the URL. */
  worktree?: WorktreeTarget
  epic?: string
  /** Commit SHAs the view is narrowed to. */
  commits?: string[]
  /** A ticket's canonical address; the view narrows to the returned commits that carry it. */
  ticket?: string
  /** The selected file's path. */
  file?: string
}

const KEYS = {
  project: "project",
  first: "base",
  second: "tip",
  epic: "epic",
  commits: "commits",
  ticket: "ticket",
  file: "file",
  worktree: "worktree",
} as const

export function parseDiffUrl(params: URLSearchParams): DiffUrl {
  const get = (k: string) => params.get(k) ?? ""
  const worktree = parseWorktree(get(KEYS.worktree))
  const out: DiffUrl = {
    project: get(KEYS.project),
    first: worktree ? "" : get(KEYS.first),
    second: worktree ? "" : get(KEYS.second),
  }
  if (worktree) out.worktree = worktree
  if (get(KEYS.epic)) out.epic = get(KEYS.epic)
  const commits = get(KEYS.commits).split(",").filter(Boolean)
  if (commits.length) out.commits = commits
  if (get(KEYS.ticket)) out.ticket = get(KEYS.ticket)
  if (get(KEYS.file)) out.file = get(KEYS.file)
  return out
}

/** The query string without "?"; absent parts are dropped. */
export function diffQuery(url: DiffUrl): string {
  const p = new URLSearchParams()
  const set = (k: string, v: string | undefined) => v && p.set(k, v)
  set(KEYS.project, url.project)
  if (url.worktree) set(KEYS.worktree, worktreeValue(url.worktree))
  else {
    set(KEYS.first, url.first)
    set(KEYS.second, url.second)
  }
  set(KEYS.epic, url.epic)
  set(KEYS.commits, url.commits?.join(","))
  set(KEYS.ticket, url.ticket)
  set(KEYS.file, url.file)
  return p.toString()
}

/** The project part of a `project:name` address. */
export const projectOf = (address: string) => address.slice(0, address.indexOf(":"))

/** What "View diff" opens: an epic's range, narrowed to a ticket's commits when `ticket` is set. The link and the Show diff action both build it here. */
export const viewDiffUrl = (p: {
  epic: string
  base?: string
  tip?: string
  ticket?: string
}): DiffUrl => ({
  project: projectOf(p.epic),
  first: p.base ?? "",
  second: p.tip ?? "",
  epic: p.epic,
  ticket: p.ticket,
})

/** A worktree diff of one target. */
export const worktreeUrl = (project: string, worktree: WorktreeTarget): DiffUrl => ({
  project,
  first: "",
  second: "",
  worktree,
})
