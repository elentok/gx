import { apiFetch } from "../apiFetch"
import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  type DesignDraft,
  type DesignDraftItem,
  type DesignDraftSaveRequest,
  type DesignDraftSaveResult,
  type DesignLastProject,
} from "../generated/server"
import { WriteError } from "../useWrite"
import type { RemoveDraftImage, UploadDraftImage } from "./draftImageApi"

/** What a draft call came to. A refusal is a value the form branches on; transport failures throw. */
export type DraftResult =
  | { kind: "ok"; draft: DesignDraft }
  | { kind: "deleted" }
  | { kind: "conflict" }
  | { kind: "gone" }

export type ListDrafts = () => Promise<DesignDraftItem[]>
export type ReadDraft = (id: string) => Promise<DraftResult>
export type SaveDraft = (body: DesignDraftSaveRequest) => Promise<DraftResult>
export type DeleteDraft = (id: string) => Promise<DraftResult>
export type GetLastProject = () => Promise<string>
export type SetLastProject = (project: string) => Promise<void>

/** The draft calls the form makes; the prop defaults to the real ones. */
export interface DraftsClient {
  list: ListDrafts
  read: ReadDraft
  save: SaveDraft
  remove: DeleteDraft
  getLastProject: GetLastProject
  setLastProject: SetLastProject
  uploadImage: UploadDraftImage
  removeImage: RemoveDraftImage
}

const BASE = "/v1/designs/drafts"

async function request(path: string, body?: unknown): Promise<string> {
  const res = await apiFetch(
    path,
    body === undefined
      ? undefined
      : {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
  )
  const text = await res.text().catch(() => "")
  if (!res.ok) throw new WriteError(text.trim() || `HTTP ${res.status}`)
  return text
}

async function json<T>(path: string, body?: unknown): Promise<T> {
  const text = await request(path, body)
  try {
    return JSON.parse(text) as T
  } catch {
    throw new WriteError(text.trim() || "Unreadable server answer")
  }
}

function toResult(r: DesignDraftSaveResult): DraftResult {
  if (r.refused) {
    if (r.reason === ReasonDraftRevisionConflict) return { kind: "conflict" }
    if (r.reason === ReasonDraftGone) return { kind: "gone" }
    throw new WriteError(r.reason || "Draft refused")
  }
  if (r.deleted) return { kind: "deleted" }
  if (r.draft) return { kind: "ok", draft: r.draft }
  throw new WriteError("Empty draft answer")
}

export const listDrafts: ListDrafts = () => json<DesignDraftItem[]>(BASE)

export const readDraft: ReadDraft = async (id) =>
  toResult(await json<DesignDraftSaveResult>(`${BASE}/read?id=${encodeURIComponent(id)}`))

export const saveDraft: SaveDraft = async (body) =>
  toResult(await json<DesignDraftSaveResult>(`${BASE}/save`, body))

/** The most recent draft, or null when there is none (or it cannot be read). */
export async function openLatest(client: {
  list: ListDrafts
  read: ReadDraft
}): Promise<DesignDraft | null> {
  const [latest] = await client.list()
  const res = latest ? await client.read(latest.id) : undefined
  return res?.kind === "ok" ? res.draft : null
}

export const deleteDraft: DeleteDraft = async (id) =>
  toResult(await json<DesignDraftSaveResult>(`${BASE}/delete`, { id }))

export const getLastProject: GetLastProject = async () =>
  (await json<DesignLastProject>(`${BASE}/last-project`)).project

export const setLastProject: SetLastProject = async (project) => {
  await request(`${BASE}/last-project`, { project } satisfies DesignLastProject)
}

const MAX_LABEL = 40
const LABEL_WORDS = 5

type Labelled = Pick<DesignDraftItem, "name" | "slug" | "idea">

/** The draft's row label: name, else slug, else the idea's first words; no draft is a blank new one. */
export function draftLabel(d?: Labelled | null): string {
  if (!d) return "New draft"
  const name = d.name?.trim()
  if (name) return name
  const slug = d.slug.trim()
  if (slug) return slug
  const words = d.idea.trim().split(/\s+/).filter(Boolean).slice(0, LABEL_WORDS).join(" ")
  if (!words) return "Untitled draft"
  return words.length > MAX_LABEL ? `${words.slice(0, MAX_LABEL)}…` : words
}

/** "just now", "N min ago", "N h ago", "N d ago". */
export function timeAgo(editedAt: string, now: Date = new Date()): string {
  const minutes = Math.floor((now.getTime() - Date.parse(editedAt)) / 60_000)
  if (!(minutes >= 1)) return "just now"
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h ago`
  return `${Math.floor(hours / 24)} d ago`
}
