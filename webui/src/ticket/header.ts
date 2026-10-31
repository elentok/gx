import { RoundOpen, type TicketDetail } from "../generated/server";
import { NO_COST, formatCost, formatDuration, toneOfStatus } from "../epic/rows";
import type { Tone } from "../ui/tone";

export const isRunning = (d: TicketDetail) => d.status === "claimed";

// The server sends a paused ticket under its legacy status words, never "paused". needs-repair is
// "attention" on the epic page, so the badge keeps its own Paused wording for it.
const PAUSED_STATUSES = ["needs-answer", "needs-repair"];

export function badgeOf(d: TicketDetail): { tone: Tone; label: string } {
  if (PAUSED_STATUSES.includes(d.status)) {
    return { tone: "paused", label: d.pause_label ? `Paused · ${d.pause_label}` : "Paused" };
  }
  if (isRunning(d) || d.status === "done") return { tone: toneOfStatus(d.status), label: d.status };
  if (d.error_reason) return { tone: "deadlocked", label: d.status };
  const waiting = d.latest?.outcome === RoundOpen && !d.answered;
  return { tone: waiting ? "waiting" : "muted", label: d.status };
}

// A running ticket counts up from claimed_at; every other ticket shows the elapsed time the server sent.
function elapsedOf(d: TicketDetail, now: number): number | undefined {
  const s = d.stats;
  if (isRunning(d) && s?.claimed_at) return Math.max(0, (now - Date.parse(s.claimed_at)) / 1000);
  return s?.elapsed;
}

// Only the values the ticket has; a map ticket has no stat line.
export function statValues(d: TicketDetail, now: number): string[] {
  if (d.map_epic || d.parse_error) return [];
  const elapsed = elapsedOf(d, now);
  const { cost, had_codex: hadCodex, context_window: ctx } = d.stats ?? {};
  return [
    d.type,
    elapsed !== undefined && formatDuration(elapsed),
    hadCodex ? `Cost ${NO_COST}` : cost !== undefined && formatCost(cost),
    ctx !== undefined && `${Math.round(ctx / 1000)}K context`,
  ].filter((v): v is string => !!v);
}
