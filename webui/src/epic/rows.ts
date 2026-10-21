import type { EpicDetail, EpicDetailRow } from "./detail";
import type { Segment } from "../ui/progress-bar/SegmentedBar";
import type { Tone } from "../ui/tone";
import { epicBadge } from "../view";

type Bucket = "done" | "running" | "needs-answer" | "waiting" | "attention";

const BUCKETS: Record<string, Bucket> = {
  done: "done",
  cancelled: "done",
  claimed: "running",
  "needs-answer": "needs-answer",
  "needs-repair": "attention",
  error: "attention",
};

// Everything else (blocked, open, draft, waiting-for-children) waits.
const bucketOf = (row: EpicDetailRow): Bucket => BUCKETS[row.rendered_status] ?? "waiting";

const BUCKET_TONES: Record<Bucket, Tone> = {
  done: "muted",
  running: "running",
  "needs-answer": "parked",
  waiting: "waiting",
  attention: "deadlocked",
};

export const isCancelled = (row: EpicDetailRow) => row.rendered_status === "cancelled";
export const isFolded = (row: EpicDetailRow) => bucketOf(row) === "done";
export const toneOf = (row: EpicDetailRow): Tone => BUCKET_TONES[bucketOf(row)];

export function segmentsOf(rows: EpicDetailRow[]): Segment[] {
  return (Object.keys(BUCKET_TONES) as Bucket[]).map((key) => ({
    key,
    tone: BUCKET_TONES[key],
    count: rows.filter((r) => bucketOf(r) === key).length,
  }));
}

export function formatDuration(seconds: number): string {
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const rest = m % 60;
  return rest ? `${Math.floor(m / 60)}h ${rest}m` : `${m / 60}h`;
}

export const formatCost = (dollars: number) => `$${dollars.toFixed(2)}`;

export function statSentence(d: EpicDetail): string {
  const done = `${d.done} of ${d.total} done`;
  if (d.kind === "map") return done;
  return [
    d.state && epicBadge(d.state),
    done,
    d.wall_clock !== undefined && `took ${formatDuration(d.wall_clock)}`,
    d.cost_total !== undefined && formatCost(d.cost_total),
    d.agent_time !== undefined && `${formatDuration(d.agent_time)} of agent time`,
  ]
    .filter(Boolean)
    .join(" · ");
}

// The dim detail line under a row: status word, elapsed time and cost. A map epic's rows carry the status only.
export function detailLine(row: EpicDetailRow, kind: string): string {
  const parts: (string | false | undefined)[] = [row.rendered_status];
  if (kind !== "map") {
    parts.push(row.elapsed !== undefined && formatDuration(row.elapsed), row.cost !== undefined && formatCost(row.cost));
  }
  return parts.filter(Boolean).join(" · ");
}

// A folded row's dim tail: elapsed time (or a muted "cancelled" word), then cost.
export function foldedTail(row: EpicDetailRow, kind: string): string {
  const parts: (string | false | undefined)[] = [];
  if (isCancelled(row)) parts.push("cancelled");
  else if (kind !== "map") parts.push(row.elapsed !== undefined && formatDuration(row.elapsed));
  if (kind !== "map") parts.push(row.cost !== undefined && formatCost(row.cost));
  return parts.filter(Boolean).join(" · ");
}
