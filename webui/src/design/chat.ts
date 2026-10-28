import { useCallback } from "react";
import type { Connection } from "../connection";
import type { SessionChatResponse } from "../generated/server";
import { fetchJson, useDetail, type DetailResult } from "../ticket/detail";

export type FetchChat = (sessionId: string) => Promise<DetailResult<SessionChatResponse>>;

export const fetchChat: FetchChat = (sessionId) =>
  fetchJson(`/v1/sessions/chat?session=${encodeURIComponent(sessionId)}`, "session chat");

/** The chat refetches on any event addressed to the session: its chat grew (session-chat-changed) or it changed (session-changed). */
export const useSessionChat = (sessionId: string | undefined, conn: Connection, fetcher: FetchChat = fetchChat) => {
  // Stable, or useDetail refetches on every render. No session id (none, or not Claude): nothing to fetch.
  const fetchOrNone = useCallback<FetchChat>(
    (id) => (id ? fetcher(id) : Promise.resolve({ kind: "not-found" })),
    [fetcher],
  );
  return useDetail(sessionId ?? "", conn, fetchOrNone);
};

/** Names the pending round: a new round (or none) is a different key. */
export const roundKeyOf = (chat: SessionChatResponse | null | undefined) =>
  (chat?.round ?? []).map((c) => `${c.label}:${c.title}`).join("|");
