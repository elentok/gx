import { apiFetch } from "../apiFetch"
import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  type DesignDraft,
  type DesignDraftItem,
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
  | { kind: "gone" }

export type ListDrafts = () => Promise<DesignDraftItem[]>
export type ReadDraft = (id: string) => Promise<DraftResult>
/** What the form saves: the text request plus the slug and project, which `saveDraft` writes apart from it. */
export type SaveDraftBody = DesignDraftSaveRequest &
  Pick<DesignDraftTargetRequest, "project" | "slug">
export type SaveDraft = (body: SaveDraftBody) => Promise<DraftResult>
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

/**
 * The form saves all its fields in one call. The server keeps the slug and project apart from the
 * text so a stale text save cannot write them back: they go through the revision-free target write,
 * before the text save of an existing draft and after the creating save of a new one.
 */
export const saveDraft: SaveDraft = async ({ project, slug, ...text }) => {
  const setTarget = async (id: string) =>
    toResult(
      await json<DesignDraftSaveResult>(`${BASE}/target`, {
        id,
        project,
        slug,
        keep_edited_at: text.keep_edited_at,
      } satisfies DesignDraftTargetRequest),
    )
  const saveText = async () => toResult(await json<DesignDraftSaveResult>(`${BASE}/save`, text))
  if (!text.id) {
    const created = await saveText()
    return created.kind === "ok" && (project || slug) ? setTarget(created.draft.id) : created
  }
  const moved = await setTarget(text.id)
  return moved.kind === "ok" ? saveText() : moved
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

export const getLastProject: GetLastProject = async () =>
  (await json<DesignLastProject>(`${BASE}/last-project`)).project

export const setLastProject: SetLastProject = async (project) => {
  await request(`${BASE}/last-project`, { project } satisfies DesignLastProject)
}

/** The draft's row label, as the server shipped it; no draft is a blank new one. */
export function draftLabel(d?: Pick<DesignDraftItem, "label"> | null): string {
  return d ? d.label : "New draft"
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
