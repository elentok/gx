import { RoundOpen, type TicketDetail } from "../generated/server";
import { formatCost, formatDuration } from "../epic/rows";
import type { Tone } from "../ui/tone";

export const isRunning = (d: TicketDetail) => d.status === "claimed";

export function badgeOf(d: TicketDetail): { tone: Tone; label: string } {
  if (d.status === "paused") {
    return { tone: "paused", label: d.pause_label ? `Paused · ${d.pause_label}` : "Paused" };
  }
  if (isRunning(d)) return { tone: "running", label: d.status };
  if (d.status === "done") return { tone: "done", label: d.status };
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
  if (d.map_epic) return [];
  const elapsed = elapsedOf(d, now);
  const { cost, context_window: ctx } = d.stats ?? {};
  return [
    d.type,
    elapsed !== undefined && formatDuration(elapsed),
    cost !== undefined && formatCost(cost),
    ctx !== undefined && `${Math.round(ctx / 1000)}K context`,
  ].filter((v): v is string => !!v);
}
