// The Diff page's address: every part of the view lives in the query, so a reload or a pasted link
// shows the same view. This module owns the query names; nothing else spells them.

export interface DiffUrl {
  project: string
  /** The base ref, as typed. */
  first: string
  /** The tip ref, as typed. */
  second: string
  epic?: string
  /** Commit SHAs the view is narrowed to. */
  commits?: string[]
  /** The selected file's path. */
  file?: string
}

const KEYS = {
  project: "project",
  first: "base",
  second: "tip",
  epic: "epic",
  commits: "commits",
  file: "file",
} as const

export function parseDiffUrl(params: URLSearchParams): DiffUrl {
  const get = (k: string) => params.get(k) ?? ""
  const out: DiffUrl = {
    project: get(KEYS.project),
    first: get(KEYS.first),
    second: get(KEYS.second),
  }
  if (get(KEYS.epic)) out.epic = get(KEYS.epic)
  const commits = get(KEYS.commits).split(",").filter(Boolean)
  if (commits.length) out.commits = commits
  if (get(KEYS.file)) out.file = get(KEYS.file)
  return out
}

/** The query string without "?"; absent parts are dropped. */
export function diffQuery(url: DiffUrl): string {
  const p = new URLSearchParams()
  const set = (k: string, v: string | undefined) => v && p.set(k, v)
  set(KEYS.project, url.project)
  set(KEYS.first, url.first)
  set(KEYS.second, url.second)
  set(KEYS.epic, url.epic)
  set(KEYS.commits, url.commits?.join(","))
  set(KEYS.file, url.file)
  return p.toString()
}
