import { formatDuration } from "../epic/rows";
import { RunAgentGoneAfterRestart, RunPaused, RunRunning, StartNewSession, type DetailRun } from "../generated/server";

export const formatStart = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

// The server sends "agent-lost" and the like; the page reads them as words.
function outcomeOf(r: DetailRun): string {
  if (r.outcome === RunPaused) return r.pause_label ? `paused · ${r.pause_label}` : "paused";
  if (r.outcome === RunAgentGoneAfterRestart) return "agent gone after server restart, no work to keep, back in the queue";
  return r.outcome.replaceAll("-", " ");
}

// A running run counts up from its start; a run with no duration (agent lost) shows none.
function durationOf(r: DetailRun, now: number): number | undefined {
  if (r.outcome === RunRunning) return Math.max(0, (now - Date.parse(r.start)) / 1000);
  return r.duration;
}

export function runLine(r: DetailRun, now: number): string {
  const duration = durationOf(r, now);
  return [
    formatStart(r.start),
    duration !== undefined && formatDuration(duration),
    outcomeOf(r),
    r.start_kind === StartNewSession ? "new session" : "same session",
  ]
    .filter((v) => v !== false)
    .join(" · ");
}
