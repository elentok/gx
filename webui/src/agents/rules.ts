import { SessionBlocked, SessionExited, type SessionInfo } from "../generated/server";

// Ports of ui/agents/sidebar.go: orderedSessions, liveCount and isActionable.

/** Sessions grouped by project, projects in order of their first session, creation order inside each. */
export function groupSessions(sessions: SessionInfo[]): { project: string; sessions: SessionInfo[] }[] {
  const groups = new Map<string, SessionInfo[]>();
  for (const s of sessions) {
    const group = groups.get(s.project);
    if (group) group.push(s);
    else groups.set(s.project, [s]);
  }
  return [...groups].map(([project, list]) => ({ project, sessions: list }));
}

export const liveCount = (sessions: SessionInfo[]) => sessions.filter((s) => s.status.state !== SessionExited).length;

/** A Blocked session always waits on the person, with or without a recorded reason. */
export const isActionable = (s: SessionInfo) => !!s.actionable.reason || s.status.state === SessionBlocked;

export const actionableCount = (sessions: SessionInfo[]) => sessions.filter(isActionable).length;
