import { useCallback } from "react"
import type { Connection } from "../connection"
import {
  CardSourceRound,
  type QuestionCard,
  type SessionChatResponse,
  type SessionRoundCard,
} from "../generated/server"
import { fetchJson, fetchJsonWithMessage, useDetail, type DetailResult } from "../ticket/detail"

export type FetchChat = (sessionId: string) => Promise<DetailResult<SessionChatResponse>>

export const fetchChat: FetchChat = (sessionId) =>
  fetchJson(`/v1/sessions/chat?session=${encodeURIComponent(sessionId)}`, "session chat")

/** Decodes a Codex session's archived rollout; only the Decode button calls it. */
export const fetchArchive: FetchChat = (sessionId) =>
  fetchJson(`/v1/sessions/chat/archive?session=${encodeURIComponent(sessionId)}`, "archived chat")

/** One subagent's page; a 404 carries the server's message ("no such subagent", or the unknown-session one). */
export const fetchSubagentChat = (sessionId: string, callId: string) =>
  fetchJsonWithMessage<SessionChatResponse>(
    `/v1/sessions/chat?session=${encodeURIComponent(sessionId)}&subagent=${encodeURIComponent(callId)}`,
    "subagent chat",
  )

/** The harnesses whose transcripts gx reads into a chat. */
export const hasChat = (harness: string | undefined) => harness === "claude" || harness === "codex"

/** Codex skills start with `$`: a Codex draft that starts with `/gx-` gets this line. Claude never does. */
export const codexSkillHint = (harness: string | undefined, draft: string) =>
  harness === "codex" && draft.startsWith("/gx-")
    ? "Codex skills start with $. Type $gx-... instead."
    : undefined

// wire.EventSessionSubagentChanged: addressed to the root session; `client` names the tab it is for.
export const EventSessionSubagentChanged = "session-subagent-changed"

/**
 * The chat refetches on any event addressed to the session: its chat grew (session-chat-changed) or it
 * changed (session-changed). A subagent event counts only for this tab: every stream gets every tab's.
 */
export const useSessionChat = (
  sessionId: string | undefined,
  conn: Connection,
  fetcher: FetchChat = fetchChat,
) => {
  // Stable, or useDetail refetches on every render. No session id (none, or a harness with no chat): nothing to fetch.
  const fetchOne = useCallback(
    (): ReturnType<FetchChat> =>
      sessionId ? fetcher(sessionId) : Promise.resolve({ kind: "not-found" }),
    [fetcher, sessionId],
  )
  const accepts = useCallback(
    (address: string | undefined, type: string, data: Record<string, unknown>) =>
      address === sessionId &&
      (type !== EventSessionSubagentChanged || data?.client === conn.clientId),
    [sessionId, conn],
  )
  // The key is not the session id, or useDetail would refetch on the address match before it asks `accepts`.
  return useDetail<SessionChatResponse>(
    `chat\n${sessionId ?? ""}`,
    conn,
    fetchOne,
    undefined,
    undefined,
    accepts,
  )
}

const roundCardOf = (chat: SessionChatResponse | null | undefined) =>
  chat?.questions.find((c) => c.source === CardSourceRound)

/** A card's questions as the sheet's cards. A question with no label is named by its place, so answers never collide. */
export const sheetCardsOf = (card: QuestionCard | undefined): SessionRoundCard[] =>
  (card?.questions ?? []).map((q, i) => ({
    label: q.label || String(i + 1),
    title: q.title,
    body: q.body,
    kind: q.kind,
    yes: q.yes,
    no: q.no,
    options: q.options?.map((o) => ({
      id: o.id,
      label: o.label,
      consequence: o.consequence,
      text: o.text,
    })),
    recommended: q.recommended,
    links: q.links,
    note: q.note,
  }))

/** The printed round's questions, as the sheet's cards; empty when no round is open. */
export const roundOf = (chat: SessionChatResponse | null | undefined): SessionRoundCard[] =>
  sheetCardsOf(roundCardOf(chat))

/** Names the pending round by the server's card id: a new round (or none) is a different key. */
export const roundKeyOf = (chat: SessionChatResponse | null | undefined) =>
  roundCardOf(chat)?.id ?? ""
