import { useCallback } from "react";
import type { Connection } from "../connection";
import type { SessionChatResponse } from "../generated/server";
import { fetchJson, useDetail, type DetailResult } from "../ticket/detail";

export type FetchChat = (sessionId: string) => Promise<DetailResult<SessionChatResponse>>;

export const fetchChat: FetchChat = (sessionId) =>
  fetchJson(`/v1/sessions/chat?session=${encodeURIComponent(sessionId)}`, "session chat");

/** Decodes a Codex session's archived rollout; only the Decode button calls it. */
export const fetchArchive: FetchChat = (sessionId) =>
  fetchJson(`/v1/sessions/chat/archive?session=${encodeURIComponent(sessionId)}`, "archived chat");

/** The harnesses whose transcripts gx reads into a chat. */
export const hasChat = (harness: string | undefined) => harness === "claude" || harness === "codex";

/** Codex skills start with `$`: a Codex draft that starts with `/gx-` gets this line. Claude never does. */
export const codexSkillHint = (harness: string | undefined, draft: string) =>
  harness === "codex" && draft.startsWith("/gx-") ? "Codex skills start with $. Type $gx-... instead." : undefined;

/** The chat refetches on any event addressed to the session: its chat grew (session-chat-changed) or it changed (session-changed). */
export const useSessionChat = (sessionId: string | undefined, conn: Connection, fetcher: FetchChat = fetchChat) => {
  // Stable, or useDetail refetches on every render. No session id (none, or a harness with no chat): nothing to fetch.
  const fetchOrNone = useCallback<FetchChat>(
    (id) => (id ? fetcher(id) : Promise.resolve({ kind: "not-found" })),
    [fetcher],
  );
  return useDetail(sessionId ?? "", conn, fetchOrNone);
};

/** Names the pending round: a new round (or none) is a different key. */
export const roundKeyOf = (chat: SessionChatResponse | null | undefined) =>
  (chat?.round ?? []).map((c) => `${c.label}:${c.title}`).join("|");
