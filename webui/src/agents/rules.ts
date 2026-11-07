import {
  SessionBlocked,
  SessionExitCrashed,
  SessionExited,
  SessionIdle,
  SessionRateLimited,
  SessionWorking,
  type SessionInfo,
} from "../generated/server";
import type { Tone } from "../ui/tone";

/** Queue and Design agents carry only a status mark: no Unread, no Actionable, no count. */
export const hasAttentionMarks = (s: SessionInfo) => !s.queue && !s.design;

/** A Blocked session always waits on the person, with or without a recorded reason. */
export const isActionable = (s: SessionInfo) => hasAttentionMarks(s) && (!!s.actionable.reason || s.status.state === SessionBlocked);

export const stateLabel = (s: SessionInfo) => s.status.state.charAt(0).toUpperCase() + s.status.state.slice(1).replace("-", " ");

// Ports of statusIcon in ui/agents/sidebar.go; Working is the animated glyph there.
export function statusMark(s: SessionInfo): { glyph: string; tone: Tone; faded?: boolean } {
  switch (s.status.state) {
    case SessionWorking:
      return { glyph: "●", tone: "running" };
    case SessionIdle:
      return { glyph: "○", tone: "done" };
    case SessionBlocked:
      return { glyph: "?", tone: "waiting" };
    case SessionRateLimited:
      return { glyph: "⏸", tone: "paused" };
    case SessionExited:
      return s.exit === SessionExitCrashed ? { glyph: "✕", tone: "deadlocked" } : { glyph: "○", tone: "muted", faded: true };
    default:
      return { glyph: "◌", tone: "paused" };
  }
}

export const actionableCount =(sessions: SessionInfo[]) => sessions.filter(isActionable).length;
