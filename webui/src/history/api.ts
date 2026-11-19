import { postJson } from "../design/open"
import type { DraftHistoryListRequest, DraftHistoryListResult } from "../generated/ops"

export type HistoryList = "chat-drafts" | "design-drafts" | "sent"

/** One of the three history lists, newest first. */
export const postHistoryList: (body: DraftHistoryListRequest) => Promise<DraftHistoryListResult> =
  postJson<DraftHistoryListResult>("/v1/draft-history/list")
