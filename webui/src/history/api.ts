import { postJson } from "../design/open"
import type {
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
