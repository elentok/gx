import { apiFetch } from "../apiFetch"
import type { SessionImageResult, SessionResult } from "../generated/server"
import { postJson, readWrite } from "./open"

/** Posts the raw bytes; resolves to the saved image id, rejects with the refusal or error message. */
export type UploadDraftImage = (draftId: string, file: Blob) => Promise<string>
export type RemoveDraftImage = (draftId: string, image: string) => Promise<unknown>

export const MSG_DRAFT_GONE = "Draft is gone"

export interface DraftImageApi {
  upload: UploadDraftImage
  remove: RemoveDraftImage
  url: (draftId: string, image: string) => string
}

/** The upload, remove and url calls of a draft kind whose image routes live under `base`. */
export function draftImageApi(base: string): DraftImageApi {
  const postRemove = postJson(`${base}/delete`)
  return {
    upload: async (draftId, file) => {
      const res = await apiFetch(`${base}?id=${encodeURIComponent(draftId)}`, {
        method: "POST",
        headers: { "Content-Type": "application/octet-stream" },
        body: file,
      })
      const result = await readWrite<SessionImageResult & SessionResult>(res)
      if (result.reason === "draft-gone") throw new Error(MSG_DRAFT_GONE)
      if (result.refused || !result.id) throw new Error(result.message || "Upload failed")
      return result.id
    },
    remove: (draftId, image) => postRemove({ id: draftId, image }),
    url: (draftId, image) =>
      `${base}?id=${encodeURIComponent(draftId)}&image=${encodeURIComponent(image)}`,
  }
}

const design = draftImageApi("/v1/designs/drafts/images")
export const uploadDraftImage = design.upload
export const removeDraftImage = design.remove
export const draftImageUrl = design.url

/** The chat drafts' image routes. */
export const chatDraftImages = draftImageApi("/v1/agents/drafts/images")
