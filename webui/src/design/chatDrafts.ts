import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  type ChatDraft,
  type ChatDraftReadRequest,
  type ChatDraftResult,
  type ChatDraftSaveRequest,
} from "../generated/server"
import { WriteError } from "../useWrite"
import { json } from "./drafts"

/** What a chat draft call came to. A refusal is a value the page branches on; transport failures throw. */
export type ChatDraftOutcome =
  | { kind: "ok"; draft: ChatDraft }
  | { kind: "deleted" }
  | { kind: "conflict" }
  /** `started` is the session id when the draft was started, else the draft was removed. */
  | { kind: "gone"; started?: string }

/** What a Start came to; a failed one leaves the draft in place with `message` saved on it. */
export type ChatStartOutcome =
  | { kind: "started"; session: string }
  | { kind: "failed"; message: string }
  | { kind: "gone"; started?: string }

/** The chat draft calls the page makes; the prop defaults to the real ones. */
export interface ChatDraftsClient {
  read: (id: string) => Promise<ChatDraftOutcome>
  save: (body: ChatDraftSaveRequest) => Promise<ChatDraftOutcome>
  remove: (id: string) => Promise<ChatDraftOutcome>
  start: (id: string) => Promise<ChatStartOutcome>
}

const BASE = "/v1/agents/drafts"

function toResult(r: ChatDraftResult): ChatDraftOutcome {
  if (r.refused) {
    if (r.reason === ReasonDraftRevisionConflict) return { kind: "conflict" }
    if (r.reason === ReasonDraftGone) return { kind: "gone", started: r.started }
    throw new WriteError(r.message || r.reason || "Draft refused")
  }
  if (r.deleted) return { kind: "deleted" }
  if (r.draft) return { kind: "ok", draft: r.draft }
  throw new WriteError("Empty draft answer")
}

const call = async (path: string, body: ChatDraftReadRequest | ChatDraftSaveRequest) =>
  toResult(await json<ChatDraftResult>(`${BASE}/${path}`, body))

function toStart(r: ChatDraftResult): ChatStartOutcome {
  if (r.refused) {
    if (r.reason === ReasonDraftGone) return { kind: "gone", started: r.started }
    return { kind: "failed", message: r.message || r.reason || "Start failed" }
  }
  if (r.started) return { kind: "started", session: r.started }
  throw new WriteError("Empty start answer")
}

export const realChatDrafts: ChatDraftsClient = {
  start: async (id) => toStart(await json<ChatDraftResult>(`${BASE}/start`, { id })),
  read: (id) => call("read", { id }),
  save: (body) => call("save", body),
  remove: (id) => call("delete", { id }),
}
