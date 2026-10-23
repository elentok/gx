import type { EpicDetail, EpicDetailRow } from "./detail";
import type { Segment } from "../ui/progress-bar/SegmentedBar";
import type { StatusKind } from "../ui/status-icon/StatusIcon";
import type { Tone } from "../ui/tone";
import { epicBadge } from "../view";

type Bucket = "done" | "running" | "asking" | "waiting" | "children" | "attention";

const BUCKETS: Record<string, Bucket> = {
  done: "done",
  cancelled: "done",
  claimed: "running",
  "needs-answer": "asking",
  "waiting-for-children": "children",
  "needs-repair": "attention",
  error: "attention",
};

// Everything else (blocked, open, draft) waits. A row waiting for children is not stuck, so it is muted.
const bucketOfStatus = (status: string): Bucket => BUCKETS[status] ?? "waiting";
const bucketOf = (row: EpicDetailRow): Bucket => bucketOfStatus(row.rendered_status);

const BUCKET_TONES: Record<Bucket, Tone> = {
  done: "done",
  running: "running",
  asking: "paused",
  waiting: "waiting",
  children: "muted",
  attention: "deadlocked",
};

const BUCKET_ICONS: Record<Bucket, StatusKind | undefined> = {
  done: "done",
  running: undefined,
  asking: "asking",
  waiting: "waiting",
  children: "children",
  attention: "attention",
};

export const iconOf = (row: EpicDetailRow) => BUCKET_ICONS[bucketOf(row)];
export const isCancelled = (row: EpicDetailRow) => row.rendered_status === "cancelled";
export const isFolded = (row: EpicDetailRow) => bucketOf(row) === "done";
export const isRunning = (row: EpicDetailRow) => bucketOf(row) === "running";
export const toneOfStatus = (status: string): Tone => BUCKET_TONES[bucketOfStatus(status)];
export const toneOf = (row: EpicDetailRow): Tone => toneOfStatus(row.rendered_status);

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

// A Waiting epic that holds no queue item is not waiting for a slot.
const stateLabel = (d: EpicDetail) => (d.state === "waiting" && !d.queued ? "Not queued" : d.state && epicBadge(d.state));

export function statValues(d: EpicDetail): string[] {
  const done = `${d.done} of ${d.total} done`;
  if (d.kind === "map") return [done];
  return [
    stateLabel(d),
    done,
    d.wall_clock === undefined ? undefined : `took ${formatDuration(d.wall_clock)}`,
    formatCost(d.cost_total),
    `${formatDuration(d.agent_time)} of agent time`,
  ].filter((v): v is string => v !== undefined && v !== "");
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

// A fork says what it forks from when that parent is not drawn above it: the parent sits in the other group,
// or is not in the epic. A fork nested under its parent in the same group needs no note.
function forkNote(row: EpicDetailRow, byAddress: Map<string, EpicDetailRow>): string | undefined {
  if (!row.parent) return undefined;
  const parent = byAddress.get(row.parent);
  return !parent || isFolded(parent) !== isFolded(row) ? `fork of ${idOf(row.parent)}` : undefined;
}

export interface RowContext {
  detail: EpicDetail;
  now: number;
  byAddress: Map<string, EpicDetailRow>;
}

export const rowContext = (detail: EpicDetail, now: number): RowContext => ({
  detail,
  now,
  byAddress: new Map(detail.rows.map((r) => [r.address, r])),
});

const joinDot = (...parts: (string | undefined)[]) => parts.filter((p): p is string => p !== undefined && p !== "").join(" · ");

// A map epic's rows carry the status only, so only a loop epic has elapsed time and cost to show.
function elapsedAndCost(row: EpicDetailRow, elapsed: number | undefined, ctx: RowContext): (string | undefined)[] {
  if (ctx.detail.kind === "map") return [];
  return [elapsed === undefined ? undefined : formatDuration(elapsed), row.cost === undefined ? undefined : formatCost(row.cost)];
}

// The dim detail line under a row: status word, elapsed time, cost, then the blockers and fork note.
export function detailLine(row: EpicDetailRow, ctx: RowContext): string {
  return joinDot(
    row.rendered_status,
    ...elapsedAndCost(row, elapsedOf(row, ctx.now), ctx),
    row.blockers?.length ? blockerNote(row.blockers) : undefined,
    forkNote(row, ctx.byAddress),
  );
}

// A folded row's dim tail: elapsed time (or a muted "cancelled" word), then cost, then the fork note.
export function foldedTail(row: EpicDetailRow, ctx: RowContext): string {
  const [elapsed, cost] = elapsedAndCost(row, row.elapsed, ctx);
  return joinDot(isCancelled(row) ? "cancelled" : elapsed, cost, forkNote(row, ctx.byAddress));
}

export const MAX_INDENT = 2;

// Rows of one group (live or folded) in display order: each fork follows its parent, indented one level per
// generation up to MAX_INDENT. A row whose parent is in the other group or absent starts at the top level.
function nestGroup(rows: EpicDetailRow[], inGroup: (r: EpicDetailRow) => boolean): { row: EpicDetailRow; indent: number }[] {
  const group = rows.filter(inGroup);
  const addresses = new Set(group.map((r) => r.address));
  const children = new Map<string, EpicDetailRow[]>();
  const roots: EpicDetailRow[] = [];
  for (const r of group) {
    if (r.parent && addresses.has(r.parent)) children.set(r.parent, [...(children.get(r.parent) ?? []), r]);
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

export const nestLive = (rows: EpicDetailRow[]) => nestGroup(rows, (r) => !isFolded(r));
export const nestFolded = (rows: EpicDetailRow[]) => nestGroup(rows, isFolded);
