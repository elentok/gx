import { apiFetch } from "../apiFetch"
import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  type DesignDraft,
  type DesignDraftItem,
  type DesignDraftLeaseRequest,
  type DesignDraftSaveRequest,
  type DesignDraftSaveResult,
  type DesignDraftTargetRequest,
  type DesignLastProject,
} from "../generated/server"
import { WriteError } from "../useWrite"
import type { RemoveDraftImage, UploadDraftImage } from "./draftImageApi"

/** What a draft call came to. A refusal is a value the form branches on; transport failures throw. */
export type DraftResult =
  | { kind: "ok"; draft: DesignDraft }
  | { kind: "deleted" }
  | { kind: "conflict" }
  /** `started` is the Design address when the draft was started, else the draft was removed. */
  | { kind: "gone"; started?: string }

export type ListDrafts = () => Promise<DesignDraftItem[]>
export type ReadDraft = (id: string) => Promise<DraftResult>
export type SaveDraft = (body: DesignDraftSaveRequest) => Promise<DraftResult>
export type SaveDraftFields = (body: DesignDraftTargetRequest) => Promise<DraftResult>
/** What a new draft starts with; the text, slug and project are all optional. */
export interface NewDraft {
  workspace?: string
  project?: string
  slug?: string
  idea?: string
}
export type CreateDraft = (init?: NewDraft) => Promise<string>
export type DeleteDraft = (id: string) => Promise<DraftResult>
/** Renews the open page's lease; throws when the draft is gone or the call fails. */
export type PingDraft = (id: string) => Promise<void>
export type RemoveDraftIfEmpty = (id: string) => Promise<void>
export type GetLastProject = () => Promise<string>
export type SetLastProject = (project: string) => Promise<void>

/** The draft calls the form makes; the prop defaults to the real ones. */
export interface DraftsClient {
  list: ListDrafts
  read: ReadDraft
  save: SaveDraft
  saveFields: SaveDraftFields
  remove: DeleteDraft
  ping: PingDraft
  removeIfEmpty: RemoveDraftIfEmpty
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
    if (r.reason === ReasonDraftGone) return { kind: "gone", started: r.started }
    throw new WriteError(r.reason || "Draft refused")
  }
  if (r.deleted) return { kind: "deleted" }
  if (r.draft) return { kind: "ok", draft: r.draft }
  throw new WriteError("Empty draft answer")
}

export const listDrafts: ListDrafts = () => json<DesignDraftItem[]>(BASE)

export const readDraft: ReadDraft = async (id) =>
  toResult(await json<DesignDraftSaveResult>(`${BASE}/read?id=${encodeURIComponent(id)}`))

/** The text save, with its revision. The slug and project never travel with it, so a stale text save cannot write them back. */
export const saveDraft: SaveDraft = async (body) =>
  toResult(await json<DesignDraftSaveResult>(`${BASE}/save`, body))

/** The revision-free write of the slug and/or project; only a present field is written. */
export const saveDraftFields: SaveDraftFields = async (body) =>
  toResult(await json<DesignDraftSaveResult>(`${BASE}/target`, body))

/** A new draft in `workspace`; its id is the page's address. The project and slug are a second, revision-free write that may fail without losing the draft. */
export const createDraft: CreateDraft = async ({ workspace, project, slug, idea } = {}) => {
  const res = await saveDraft({
    idea: idea ?? "",
    revision: 0,
    ...(workspace ? { workspace } : {}),
  })
  if (res.kind !== "ok") throw new WriteError("The draft was not created")
  const id = res.draft.id
  if (project || slug) {
    await saveDraftFields({
      id,
      ...(project && { project }),
      ...(slug && { slug }),
    }).catch(() => {})
  }
  return id
}

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

export const pingDraft: PingDraft = async (id) => {
  const res = await json<DesignDraftSaveResult>(`${BASE}/lease`, {
    id,
  } satisfies DesignDraftLeaseRequest)
  if (res.refused) throw new WriteError(res.reason || "Draft refused")
}

/** The server refuses when text arrived meanwhile, which is not an error here. */
export const removeDraftIfEmpty: RemoveDraftIfEmpty = async (id) => {
  await request(`${BASE}/remove-if-empty`, { id } satisfies DesignDraftLeaseRequest)
}

export const getLastProject: GetLastProject = async () =>
  (await json<DesignLastProject>(`${BASE}/last-project`)).project

export const setLastProject: SetLastProject = async (project) => {
  await request(`${BASE}/last-project`, { project } satisfies DesignLastProject)
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
