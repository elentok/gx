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

export const idOf = (address: string) => address.slice(address.lastIndexOf("/") + 1);

// A running row counts up from claimed_at; every other row shows the elapsed time the server sent.
function elapsedOf(row: EpicDetailRow, now: number): number | undefined {
  if (bucketOf(row) === "running" && row.claimed_at) {
    return Math.max(0, (now - Date.parse(row.claimed_at)) / 1000);
  }
  return row.elapsed;
}

// Own-epic blockers are bare numbers and cross-epic ones are "epic/NN", so the ids print as they came.
function blockerNote(blockers: string[]): string {
  const head = blockers.slice(0, -1).join(", ");
  return `after ${head ? `${head} and ` : ""}${blockers[blockers.length - 1]}`;
}

// A fork says what it forks from when that parent is not drawn above it: the fork is folded, or its parent is.
function forkNote(row: EpicDetailRow, byAddress: Map<string, EpicDetailRow>): string | undefined {
  if (!row.parent) return undefined;
  const parent = byAddress.get(row.parent);
  return isFolded(row) || (parent && isFolded(parent)) ? `fork of ${idOf(row.parent)}` : undefined;
}

export interface RowContext {
  kind: string;
  now: number;
  byAddress: Map<string, EpicDetailRow>;
}

export const rowContext = (detail: EpicDetail, now: number): RowContext => ({
  kind: detail.kind,
  now,
  byAddress: new Map(detail.rows.map((r) => [r.address, r])),
});

// The dim detail line under a row: status word, elapsed time, cost, then the blockers and fork note.
// A map epic's rows carry the status only.
export function detailLine(row: EpicDetailRow, ctx: RowContext): string {
  const parts: (string | false | undefined)[] = [row.rendered_status];
  if (ctx.kind !== "map") {
    const elapsed = elapsedOf(row, ctx.now);
    parts.push(elapsed !== undefined && formatDuration(elapsed), row.cost !== undefined && formatCost(row.cost));
  }
  parts.push(row.blockers?.length ? blockerNote(row.blockers) : undefined, forkNote(row, ctx.byAddress));
  return parts.filter(Boolean).join(" · ");
}

// A folded row's dim tail: elapsed time (or a muted "cancelled" word), then cost, then the fork note.
export function foldedTail(row: EpicDetailRow, ctx: RowContext): string {
  const parts: (string | false | undefined)[] = [];
  if (isCancelled(row)) parts.push("cancelled");
  else if (ctx.kind !== "map") parts.push(row.elapsed !== undefined && formatDuration(row.elapsed));
  if (ctx.kind !== "map") parts.push(row.cost !== undefined && formatCost(row.cost));
  parts.push(forkNote(row, ctx.byAddress));
  return parts.filter(Boolean).join(" · ");
}

export const MAX_INDENT = 2;

// Live rows in display order: each fork follows its parent, indented one level per generation up to MAX_INDENT.
// A row whose parent is folded or absent starts at the top level.
export function nestLive(rows: EpicDetailRow[]): { row: EpicDetailRow; indent: number }[] {
  const live = rows.filter((r) => !isFolded(r));
  const liveAddresses = new Set(live.map((r) => r.address));
  const children = new Map<string, EpicDetailRow[]>();
  const roots: EpicDetailRow[] = [];
  for (const r of live) {
    if (r.parent && liveAddresses.has(r.parent)) children.set(r.parent, [...(children.get(r.parent) ?? []), r]);
    else roots.push(r);
  }
  const out: { row: EpicDetailRow; indent: number }[] = [];
  const walk = (row: EpicDetailRow, depth: number) => {
    out.push({ row, indent: Math.min(depth, MAX_INDENT) });
    children.get(row.address)?.forEach((c) => walk(c, depth + 1));
  };
  roots.forEach((r) => walk(r, 0));
  return out;
}
