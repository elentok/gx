import { postJson } from "../design/open"
import type {
  ComposerPickRequest,
  ComposerPickResult,
  DesignDraftPickResult,
  DraftHistoryListRequest,
  DraftHistoryListResult,
  DraftHistoryPickRequest,
} from "../generated/ops"

export type HistoryList = "chat-drafts" | "design-drafts" | "sent"

export type ListHistory = (body: DraftHistoryListRequest) => Promise<DraftHistoryListResult>
export type PickDesignHistory = (body: DraftHistoryPickRequest) => Promise<DesignDraftPickResult>

/** One of the three history lists, newest first. */
export const postHistoryList: ListHistory =
  postJson<DraftHistoryListResult>("/v1/draft-history/list")

/** Puts a history entry on a Design draft. */
export const postDesignPick: PickDesignHistory = postJson<DesignDraftPickResult>(
  "/v1/designs/drafts/pick-history",
)

/** Puts a Sent messages entry into a session's composer; the call carries the composer's unsent content. */
export const postComposerPick: (body: ComposerPickRequest) => Promise<ComposerPickResult> =
  postJson<ComposerPickResult>("/v1/sessions/pick-history")
