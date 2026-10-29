import type { SessionInfo } from "../generated/server";

/** A Design's session sits on a ticket of its map epic, or on the epic itself before Chart makes the map. */
export const designOf = (s: SessionInfo) => (s.design && s.ticket ? s.ticket.split("/")[0] : undefined);

const byNewest = (a: SessionInfo, b: SessionInfo) => Date.parse(b.status_changed_at) - Date.parse(a.status_changed_at);

const byNewestRun = (a: SessionInfo, b: SessionInfo) => Date.parse(b.run_started_at) - Date.parse(a.run_started_at);

/** A Design's live sessions, latest run start first: the order designs/open picks by (server/designs.go). */
// promptable mirrors the server's SessionStatus.Promptable: Idle, or a finished turn that left a
// background task running.
export const promptable = (s: SessionInfo) =>
  s.status.state === "idle" || (s.status.state === "working" && !!s.status.background);

export const liveSessions = (address: string, sessions: SessionInfo[]) =>
  sessions.filter((s) => designOf(s) === address && s.status.state !== "exited").sort(byNewestRun);

/** The session that drives a Design: the newest live one, else the newest Exited one. */
export const designSession = (address: string, sessions: SessionInfo[]): SessionInfo | undefined =>
  liveSessions(address, sessions)[0] ?? sessions.filter((s) => designOf(s) === address).sort(byNewest)[0];

/** Whether the Design has any session, live or Exited. Removed sessions are not in the list. */
export const hasDesignSession = (address: string, sessions: SessionInfo[]) =>
  sessions.some((s) => designOf(s) === address);
