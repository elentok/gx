import type { SessionInfo, SessionRequest, SessionResult } from "../generated/server"
import { postStopSession, postTarget, rowToken, type StopSession } from "../design/open"

export type RemoveSession = (body: SessionRequest) => Promise<SessionResult>
/** Remove confirms with yes, so the server stops a live session itself. */
export const postRemoveSession: RemoveSession = postTarget("/v1/sessions/remove", true)

export const isExited = (s: SessionInfo) => s.status.state === "exited"

/** Queue agents belong to ralph-loop, so the page offers no Stop or Remove for them. */
export const canManage = (s: SessionInfo) => !s.queue

/** A plain chat: not a queue agent, and not tied to a ticket, a fix or a flow (a Design is a flow). */
export const isChat = (s: SessionInfo) => !s.queue && !s.ticket && !s.fix_of && !s.flow

/** The Exited design sessions: the ones the sidebar's bulk clean-up removes. */
export const exitedDesigns = (sessions: SessionInfo[]) =>
  sessions.filter((s) => s.design && isExited(s))

export interface SessionWrites {
  stop?: StopSession
  remove?: RemoveSession
}

/** Removes a session in one call; the server stops a live one first. */
export function removeSession(
  s: SessionInfo,
  { remove = postRemoveSession }: Pick<SessionWrites, "remove"> = {},
): Promise<SessionResult> {
  return remove({ id: s.id, state: rowToken(s) })
}
