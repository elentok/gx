import type { Connection } from "../connection";
import type { SessionChatResponse } from "../generated/server";
import { fetchJson, useDetail, type DetailResult } from "../ticket/detail";

export type FetchChat = (sessionId: string) => Promise<DetailResult<SessionChatResponse>>;

export const fetchChat: FetchChat = (sessionId) =>
  fetchJson(`/v1/sessions/chat?session=${encodeURIComponent(sessionId)}`, "session chat");

/** The chat refetches on any event addressed to the session: its chat grew (session-chat-changed) or it changed (session-changed). */
export const useSessionChat = (sessionId: string, conn: Connection, fetcher: FetchChat = fetchChat) =>
  useDetail(sessionId, conn, fetcher).detail;
