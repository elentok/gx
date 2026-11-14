import { apiFetch } from "../apiFetch"
import type { DiffFiles } from "../generated/server"

export type DiffFilesResult =
  | { kind: "files"; files: DiffFiles }
  /** The server answered 400 and named the field that does not resolve. */
  | { kind: "unknown-ref"; side: "first" | "second" }

export type FetchDiffFiles = (
  project: string,
  first: string,
  second: string,
) => Promise<DiffFilesResult>

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
