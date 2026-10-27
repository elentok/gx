import { MapStepCurrent, MapStepDone, ProgressDone } from "../generated/server";
import type { EpicDetail, EpicDetailRow } from "./detail";
import type { Tone } from "../ui/tone";
import { ticketPath } from "../hrefs";
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

export const isCancelled = (row: EpicDetailRow) => row.rendered_status === "cancelled";
export const isFolded = (row: EpicDetailRow) => bucketOf(row) === "done";
export const isRunning = (row: EpicDetailRow) => bucketOf(row) === "running";
export const toneOfStatus = (status: string): Tone => BUCKET_TONES[bucketOfStatus(status)];
export const toneOf = (row: EpicDetailRow): Tone => (isCancelled(row) ? "muted" : toneOfStatus(row.rendered_status));

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
  // A toneless part that reads one step brighter than the line (the Design step, "Publishing tickets").
  bright?: boolean;
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
  if (!d.state) return undefined;
  return { text: d.state === "waiting" && !d.queued ? "Not queued" : epicBadge(d.state), tone: STATE_TONES[d.state] };
}

// A running epic's clock counts up, so it reads plain; every other one reads "took".
export function loopStatParts(d: EpicDetail): StatPart[] {
  if (d.draft && d.address.endsWith("-impl")) {
    const n = d.rows.filter((r) => !isCancelled(r)).length;
    return [
      { text: "Publishing tickets", bright: true },
      n > 0 ? { text: `${n} ${n === 1 ? "ticket" : "tickets"} so far` } : undefined,
    ].filter((p): p is StatPart => !!p);
  }
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
  const doneStep = steps.find((s) => s.step === ProgressDone);
  const label = (doneStep?.state === MapStepDone ? doneStep : steps.find((s) => s.state === MapStepCurrent))?.label;
  const time = designTime(d, live, now);
  return [
    label ? { text: label, bright: true } : undefined,
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
// Past three blockers the rest fold into "+ N more", so the meta stays short beside the title.
const shownBlockers = 3;
function blockerNote(blockers: string[]): string {
  if (blockers.length > shownBlockers) {
    return `after ${blockers.slice(0, shownBlockers).join(", ")} + ${blockers.length - shownBlockers} more`;
  }
  const head = blockers.slice(0, -1).join(", ");
  return `after ${head ? `${head} and ` : ""}${blockers[blockers.length - 1]}`;
}

// A fork says what it forks from when that parent is not drawn above it: the parent sits in the other group,
// or is not in the epic. Only live forks nest under their parent; the done group is flat, so its forks always say.
function forkNote(row: EpicDetailRow, byAddress: Map<string, EpicDetailRow>): string | undefined {
  if (!row.parent) return undefined;
  const parent = byAddress.get(row.parent);
  return !parent || isFolded(parent) || isFolded(row) ? `fork of ${idOf(row.parent)}` : undefined;
}

export interface RowContext {
  detail: EpicDetail;
  now: number;
  byAddress: Map<string, EpicDetailRow>;
  designLive: boolean;
}

export const rowContext = (detail: EpicDetail, now: number, designLive = false): RowContext => ({
  detail,
  now,
  designLive,
  byAddress: new Map(detail.rows.map((r) => [r.address, r])),
});

const joinDot = (...parts: (string | undefined)[]) => parts.filter((p): p is string => p !== undefined && p !== "").join(" · ");

// A map epic's rows carry the status only, so only a loop epic has elapsed time and cost to show.
function elapsedAndCost(row: EpicDetailRow, elapsed: number | undefined, ctx: RowContext): (string | undefined)[] {
  if (ctx.detail.kind === "map") return [];
  return [elapsed === undefined ? undefined : formatDuration(elapsed), row.cost === undefined ? undefined : formatCost(row.cost)];
}

export type CardLook = { kind: "link" } | { kind: "note"; text?: string; answerPath?: string; fix?: boolean };

// An asking or needs-repair card stays expanded: a second row under the line holds its question and an Answer
// button, or a needs-repair fix button. Every other card is one link to its ticket.
export function cardLook(row: EpicDetailRow): CardLook {
  if (row.needs_answer) return { kind: "note", text: row.question, answerPath: ticketPath(row.address) };
  if (row.rendered_status === "needs-repair") return { kind: "note", fix: true };
  return { kind: "link" };
}

// A blocked row reads "after NN" with no status word; an open row with no blockers is "ready".
function stateOf(row: EpicDetailRow): { word?: string; after?: string } {
  const after = row.blockers?.length ? blockerNote(row.blockers) : undefined;
  const status = row.rendered_status;
  if (status === "open" && !after) return { word: "ready" };
  if (status === "blocked" && after) return { after };
  if (isRunning(row)) return { word: "running", after };
  return { word: status, after };
}

const isAsking = (row: EpicDetailRow) => !!row.needs_answer || bucketOf(row) === "asking";

// A Design row counts up only while a design session runs; its other rows carry no time or cost.
function designMeta(row: EpicDetailRow, ctx: RowContext): string {
  const { word, after } = stateOf(row);
  const open = row.rendered_status === "open" || row.rendered_status === "blocked";
  const time = isRunning(row) && ctx.designLive ? elapsedOf(row, ctx.now) : undefined;
  return joinDot(row.type, open ? (after ?? word) : word, time === undefined ? undefined : formatDuration(time));
}

// The dim meta text of a live card: state word, pause reason, elapsed time, cost, then the blockers and fork note.
export function cardMeta(row: EpicDetailRow, ctx: RowContext): string {
  if (isAsking(row)) return "needs answer";
  if (ctx.detail.kind === "map") return designMeta(row, ctx);
  const { word, after } = stateOf(row);
  return joinDot(
    word,
    row.reason,
    ...elapsedAndCost(row, elapsedOf(row, ctx.now), ctx),
    after,
    forkNote(row, ctx.byAddress),
  );
}

// A done Design row shows its claim-to-close time only, and nothing under a minute.
const designDoneTime = (row: EpicDetailRow) =>
  row.elapsed !== undefined && row.elapsed >= 60 ? formatDuration(row.elapsed) : undefined;

// A done row's dim meta: elapsed time (or "cancelled"), then cost for a loop row, then the fork note.
export function doneMeta(row: EpicDetailRow, ctx: RowContext): string {
  const [elapsed, cost] = elapsedAndCost(row, row.elapsed, ctx);
  const time = ctx.detail.kind === "map" ? designDoneTime(row) : elapsed;
  return joinDot(isCancelled(row) ? "cancelled" : time, isCancelled(row) ? undefined : cost, forkNote(row, ctx.byAddress));
}

// Done rows newest-closed first, flat; a row with no close time goes last.
export const doneRows = (rows: EpicDetailRow[]) => {
  const closed = (r: EpicDetailRow) => (r.closed_at ? Date.parse(r.closed_at) : -Infinity);
  return rows.filter(isFolded).sort((a, b) => closed(b) - closed(a));
};

// The divider text over the done cards: cancelled rows are counted apart from the done ones.
export function doneHeading(rows: EpicDetailRow[]): string {
  const cancelled = rows.filter(isCancelled).length;
  return `${rows.length - cancelled} done${cancelled > 0 ? `, ${cancelled} cancelled` : ""}`;
}

export const MAX_INDENT = 2;

// Ticket IDs compare their digit runs as numbers, so 29i9 comes before 29i10.
const byId = new Intl.Collator("en", { numeric: true });

// Rows of one group (live or folded) in display order: each fork follows its parent, indented one level per
// generation up to MAX_INDENT, and siblings go in ticket number order. A row whose parent is in the other
// group or absent starts at the top level.
function nestGroup(rows: EpicDetailRow[], inGroup: (r: EpicDetailRow) => boolean): { row: EpicDetailRow; indent: number }[] {
  const group = rows.filter(inGroup).sort((a, b) => byId.compare(idOf(a.address), idOf(b.address)));
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
