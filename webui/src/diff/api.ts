import { apiFetch } from "../apiFetch"
import type {
  DiffCommit,
  DiffCommitPatch,
  DiffCommits,
  DiffFiles,
  DiffPatch,
  DiffRange,
  DiffWorktree,
} from "../generated/server"

// The Go DiffFiles embeds DiffRange, so the JSON is flat. The generated type nests it under
// "DiffRange", which the server never sends.
export type DiffFilesBody = Omit<DiffFiles, "DiffRange"> & DiffRange

export type DiffFilesResult =
  | { kind: "files"; files: DiffFilesBody }
  /** The server answered 400 and named the field that does not resolve. */
  | { kind: "unknown-ref"; side: "first" | "second" }

/** `commits` narrows the list to the files those commits changed; it is left out for the whole range. */
export type FetchDiffFiles = (
  project: string,
  first: string,
  second: string,
  commits?: string[],
) => Promise<DiffFilesResult>

export type FetchDiffCommits = (
  project: string,
  first: string,
  second: string,
) => Promise<{ commits: DiffCommit[]; truncated: boolean }>

export const fetchDiffCommits: FetchDiffCommits = async (project, first, second) => {
  const q = new URLSearchParams({ project, first, second })
  const res = await apiFetch(`/v1/diff/commits?${q}`)
  if (!res.ok) throw new Error(`diff commits: HTTP ${res.status}`)
  const body: Omit<DiffCommits, "DiffRange"> = await res.json()
  return { commits: body.commits ?? [], truncated: !!body.truncated }
}

/** One patch per selected commit, in commit order. */
export type FetchDiffCommitPatches = (
  project: string,
  firstSha: string,
  secondSha: string,
  path: string,
  commits: string[],
) => Promise<DiffCommitPatch[]>

export const fetchDiffCommitPatches: FetchDiffCommitPatches = async (
  project,
  first,
  second,
  path,
  commits,
) => {
  const q = new URLSearchParams({ project, first, second, path, commits: commits.join(",") })
  const res = await apiFetch(`/v1/diff/patch?${q}`)
  if (!res.ok) throw new Error(`diff patch: HTTP ${res.status}`)
  const body: DiffPatch = await res.json()
  return body.commit_patches ?? []
}

/** The patch call gets the SHAs the file list resolved, so a moving branch cannot mix two ranges. */
export type FetchDiffPatch = (
  project: string,
  firstSha: string,
  secondSha: string,
  path: string,
) => Promise<string>

export const fetchDiffPatch: FetchDiffPatch = async (project, first, second, path) => {
  const q = new URLSearchParams({ project, first, second, path })
  const res = await apiFetch(`/v1/diff/patch?${q}`)
  if (!res.ok) throw new Error(`diff patch: HTTP ${res.status}`)
  const body: DiffPatch = await res.json()
  return body.patch ?? ""
}

// The generated DiffBlob is the kind enum only (the Go struct and the enum share a name), so the
// side shape is written here.
export type BlobSide = {
  kind: "absent" | "text" | "image" | "binary" | "too_large"
  size?: number
  text?: string
  image_base64?: string
}

export type DiffBlobsBody = { merge_base: string; tip: string; old: BlobSide; new: BlobSide }

/** The server finds the old path of a renamed file itself, so only the current path is sent. */
export type FetchDiffBlob = (
  project: string,
  base: string,
  tip: string,
  path: string,
) => Promise<DiffBlobsBody>

export const fetchDiffBlob: FetchDiffBlob = async (project, base, tip, path) => {
  const q = new URLSearchParams({ project, base, tip, path })
  const res = await apiFetch(`/v1/diff/blob?${q}`)
  if (!res.ok) throw new Error(`diff blob: HTTP ${res.status}`)
  return res.json()
}

/** Each call snapshots the worktree, so a refetch gets new SHAs. `target` is `<kind>:<id>`. */
export type FetchDiffWorktree = (project: string, target: string) => Promise<DiffWorktree>

export const fetchDiffWorktree: FetchDiffWorktree = async (project, target) => {
  const q = new URLSearchParams({ project, target })
  const res = await apiFetch(`/v1/diff/worktree?${q}`)
  if (!res.ok) throw new Error(`diff worktree: HTTP ${res.status}`)
  return res.json()
}

export const fetchDiffFiles: FetchDiffFiles = async (project, first, second, commits) => {
  const q = new URLSearchParams({ project, first, second })
  if (commits?.length) q.set("commits", commits.join(","))
  const res = await apiFetch(`/v1/diff/files?${q}`)
  if (res.status === 400) {
    const text = await res.text()
    if (text.startsWith("Unknown ref: ")) {
      return { kind: "unknown-ref", side: text.includes("second") ? "second" : "first" }
    }
  }
  if (!res.ok) throw new Error(`diff files: HTTP ${res.status}`)
  return { kind: "files", files: await res.json() }
}
