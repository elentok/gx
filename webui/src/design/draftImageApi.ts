import { apiFetch } from "../apiFetch"
import type { SessionImageResult, SessionResult } from "../generated/server"
import { postJson, readWrite } from "./open"

/** Posts the raw bytes; resolves to the saved image id, rejects with the refusal or error message. */
export type UploadDraftImage = (draftId: string, file: Blob) => Promise<string>
export type RemoveDraftImage = (draftId: string, image: string) => Promise<unknown>

export const MSG_DRAFT_GONE = "Draft is gone"
const BASE = "/v1/designs/drafts/images"

export const uploadDraftImage: UploadDraftImage = async (draftId, file) => {
  const res = await apiFetch(`${BASE}?id=${encodeURIComponent(draftId)}`, {
    method: "POST",
    headers: { "Content-Type": "application/octet-stream" },
    body: file,
  })
  const result = await readWrite<SessionImageResult & SessionResult>(res)
  if (result.reason === "draft-gone") throw new Error(MSG_DRAFT_GONE)
  if (result.refused || !result.id) throw new Error(result.message || "Upload failed")
  return result.id
}

const postRemove = postJson(`${BASE}/delete`)

export const removeDraftImage: RemoveDraftImage = (draftId, image) =>
  postRemove({ id: draftId, image })

export function draftImageUrl(draftId: string, image: string): string {
  return `${BASE}?id=${encodeURIComponent(draftId)}&image=${encodeURIComponent(image)}`
}
