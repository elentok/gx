import type { SessionInfo, SessionRequest, SessionResult } from "../generated/server";
import { postJson, postStopSession, stateToken, type StopSession } from "../design/open";

export type RemoveSession = (body: SessionRequest) => Promise<SessionResult>;
export const postRemoveSession: RemoveSession = postJson("/v1/sessions/remove");

export const isExited = (s: SessionInfo) => s.status.state === "exited";

/** Queue agents belong to ralph-loop, so the page offers no Stop or Remove for them. */
export const canManage = (s: SessionInfo) => !s.queue;

/** The Exited design sessions: the ones the sidebar's bulk clean-up removes. */
export const exitedDesigns = (sessions: SessionInfo[]) => sessions.filter((s) => s.design && isExited(s));

export interface SessionWrites {
  stop?: StopSession;
  remove?: RemoveSession;
}

/** Removes a session; a live one is stopped first. Returns the first refusal, else the removal. */
export async function removeSession(s: SessionInfo, { stop = postStopSession, remove = postRemoveSession }: SessionWrites = {}): Promise<SessionResult> {
  let current = s;
  if (!isExited(s)) {
    const stopped = await stop({ id: s.id, state: stateToken(s) });
    if (stopped.refused || !stopped.session) return stopped;
    current = stopped.session;
  }
  return remove({ id: current.id, state: stateToken(current) });
}
