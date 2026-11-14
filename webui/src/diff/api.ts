import { apiFetch } from "../apiFetch"
import type { DiffFiles, DiffPatch, DiffRange } from "../generated/server"

// The Go DiffFiles embeds DiffRange, so the JSON is flat. The generated type nests it under
// "DiffRange", which the server never sends.
export type DiffFilesBody = Omit<DiffFiles, "DiffRange"> & DiffRange

export type DiffFilesResult =
  | { kind: "files"; files: DiffFilesBody }
  /** The server answered 400 and named the field that does not resolve. */
  | { kind: "unknown-ref"; side: "first" | "second" }

export type FetchDiffFiles = (
  project: string,
  first: string,
  second: string,
) => Promise<DiffFilesResult>

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

export const fetchDiffFiles: FetchDiffFiles = async (project, first, second) => {
  const q = new URLSearchParams({ project, first, second })
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
