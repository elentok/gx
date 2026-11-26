import { useCallback, useRef } from "react"
import type { Connection } from "../connection"
import { EventSessionSubagentChanged, fetchSubagentChat } from "../design/chat"
import type { SessionChatResponse } from "../generated/server"
import { useDetail } from "../ticket/detail"

export type FetchSubagentChat = typeof fetchSubagentChat

/**
 * One subagent's page. The key holds the call id, so another call's page never shows while this one loads.
 * It refetches on any event addressed to the root session (chat grew, session changed) and on the
 * subagent event of its own call; a sibling call's event does not.
 */
export function useSubagentChat(
  sessionId: string,
  callId: string,
  conn: Connection,
  fetcher: FetchSubagentChat = fetchSubagentChat,
) {
  const message = useRef("")
  const fetchOne = useCallback(async () => {
    const r = await fetcher(sessionId, callId)
    message.current = r.kind === "not-found" ? r.message : ""
    return r
  }, [fetcher, sessionId, callId])
  const accepts = useCallback(
    (address: string | undefined, type: string, data: Record<string, unknown>) =>
      address === sessionId && (type !== EventSessionSubagentChanged || data?.call === callId),
    [sessionId, callId],
  )
  const state = useDetail<SessionChatResponse>(
    `${sessionId}\n${callId}`,
    conn,
    fetchOne,
    undefined,
    undefined,
    accepts,
  )
  return { ...state, message: message.current }
}
