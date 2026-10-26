import type { EpicDetail, EpicDetailRow } from "./detail";
import type { StatusKind } from "../ui/status-icon/StatusIcon";
import type { Tone } from "../ui/tone";
import { ticketHref } from "../hrefs";
import { REPAIR_HINT, epicBadge } from "../view";

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

export function formatDuration(seconds: number): string {
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const rest = m % 60;
  return rest ? `${Math.floor(m / 60)}h ${rest}m` : `${m / 60}h`;
}

export const formatCost = (dollars: number) => `$${dollars.toFixed(2)}`;

export interface StatPart {
  text: string;
  tone?: Tone;
}

const STATE_TONES: Record<NonNullable<EpicDetail["state"]>, Tone> = {
  running: "running",
  merging: "running",
  finished: "done",
  paused: "paused",
  waiting: "waiting",
  "awaiting-merge": "waiting",
};

// A draft -impl epic is still being published into; a Waiting epic that holds no queue item is not waiting for a slot.
function statePart(d: EpicDetail): StatPart | undefined {
  if (d.draft && d.address.endsWith("-impl")) return { text: "Publishing tickets", tone: "muted" };
  if (!d.state) return undefined;
  return { text: d.state === "waiting" && !d.queued ? "Not queued" : epicBadge(d.state), tone: STATE_TONES[d.state] };
}

// A running epic's clock counts up, so it reads plain; every other one reads "took".
export function loopStatParts(d: EpicDetail): StatPart[] {
  const wall =
    d.wall_clock === undefined ? undefined : `${d.state === "running" ? "" : "took "}${formatDuration(d.wall_clock)}`;
  return [
    statePart(d),
    wall && { text: wall },
    { text: formatCost(d.cost_total) },
    { text: `${formatDuration(d.agent_time)} agent time` },
  ].filter((p): p is StatPart => !!p && p.text !== "");
}

// The Design's time: the rows' claim-to-close seconds, plus the claimed row's time so far only while a session
// is live. Undefined when no row has a time.
export function designTime(d: EpicDetail, live: boolean, now: number): number | undefined {
  const times = d.rows.map((r) =>
    isRunning(r) ? (live && r.claimed_at ? Math.max(0, (now - Date.parse(r.claimed_at)) / 1000) : undefined) : r.elapsed,
  );
  const known = times.filter((t): t is number => t !== undefined);
  return known.length ? known.reduce((a, b) => a + b, 0) : undefined;
}

export function designStatParts(d: EpicDetail, live: boolean, now: number): StatPart[] {
  const steps = d.steps ?? [];
  const finished = steps.find((s) => s.step === "done")?.state === "done";
  const label = (finished ? steps.find((s) => s.step === "done") : steps.find((s) => s.state === "current"))?.label;
  const time = designTime(d, live, now);
  return [
    label ? { text: label } : undefined,
    time === undefined ? undefined : { text: formatDuration(time) },
    d.stats.cost === undefined ? undefined : { text: formatCost(d.stats.cost) },
  ].filter((p): p is StatPart => !!p);
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

// What a paused row offers under its line: an asking row its question and an Answer link, a repair-class
// row the TUI hint and no link, any other paused row a link to the ticket page where recovery lives.
export function pauseAction(row: EpicDetailRow): { hint?: string; href?: string; linkLabel?: string } {
  if (row.needs_answer) return { hint: row.question, href: ticketHref(row.address) };
  if (!row.reason) return {};
  if (row.rendered_status === "needs-repair") return { hint: REPAIR_HINT };
  return { href: ticketHref(row.address), linkLabel: "Open →" };
}

// The dim detail line under a row: status word, pause reason, elapsed time, cost, then the blockers and fork note.
export function detailLine(row: EpicDetailRow, ctx: RowContext): string {
  return joinDot(
    row.rendered_status,
    row.reason,
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
