import {
  ReasonDraftGone,
  ReasonDraftRevisionConflict,
  type ChatDraft,
  type ChatDraftCreateRequest,
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
  /** Makes a draft and gives its id; a refusal throws with the server's reason. */
  create: (init: NewChatDraft) => Promise<string>
  read: (id: string) => Promise<ChatDraftOutcome>
  save: (body: ChatDraftSaveRequest) => Promise<ChatDraftOutcome>
  remove: (id: string) => Promise<ChatDraftOutcome>
  start: (id: string) => Promise<ChatStartOutcome>
}

/** What a new chat draft starts with; the brief is empty and the other settings take the server's defaults. */
export interface NewChatDraft {
  project: string
  workspace?: string
  harness?: string
  folder?: string
}

const BASE = "/v1/agents/drafts"

async function createDraft(init: NewChatDraft): Promise<string> {
  const body: ChatDraftCreateRequest = {
    project: init.project,
    workspace: init.workspace,
    harness: init.harness ?? "",
    folder: init.folder ?? "",
    brief: "",
    name: "",
    model: "",
    effort: "",
    worktree: "",
    from_session: "",
  }
  const r = await json<ChatDraftResult>(`${BASE}/create`, body)
  if (r.refused) throw new WriteError(r.message || r.reason || "Draft refused")
  if (!r.draft) throw new WriteError("Empty draft answer")
  return r.draft.id
}

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
  create: createDraft,
  start: async (id) => toStart(await json<ChatDraftResult>(`${BASE}/start`, { id })),
  read: (id) => call("read", { id }),
  save: (body) => call("save", body),
  remove: (id) => call("delete", { id }),
}
