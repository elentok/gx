import type { EpicState, EpicSummary, FinishedEpic, QueueMode, Snapshot, SummaryTicket } from "./snapshot";
import type { Tone } from "./ui/tone";

export interface TicketLineView {
  address: string;
  id: string;
  title: string;
  tone: Tone;
  detail?: string;
  // Hash route to the ticket page; set only when a person's reply can go to the question.
  href?: string;
  hint?: string;
  state?: string;
  linkLabel?: string;
  live?: boolean;
}

export interface EpicView {
  address: string;
  name: string;
  project: string;
  /** The server's state token, which a write sends back so the server can refuse a stale view. */
  state: EpicState;
  tone: Tone;
  badge: string;
  live: boolean;
  done: number;
  total: number;
  reason?: string;
  tickets: TicketLineView[];
}

export interface FinishedView {
  address: string;
  name: string;
  project: string;
  done: number;
  total: number;
  oneOff: boolean;
  finished: string;
}

export interface SnapshotView {
  epics: EpicView[];
  recentlyFinished: FinishedView[];
}

// order is the display order; Array.prototype.sort is stable, so the server's
// order holds within a state. The labels must match epicStateLabels in
// ui/tickets/queue_header.go.
const STATES: Record<EpicState, { order: number; tone: Tone; label: string }> = {
  parked: { order: 0, tone: "parked", label: "Parked" },
  deadlocked: { order: 1, tone: "deadlocked", label: "Deadlocked" },
  "merge-blocked": { order: 2, tone: "parked", label: "Merge blocked" },
  running: { order: 3, tone: "running", label: "Running" },
  merging: { order: 4, tone: "running", label: "Merging" },
  waiting: { order: 5, tone: "waiting", label: "Waiting for a slot" },
  "awaiting-merge": { order: 6, tone: "muted", label: "Awaiting merge" },
  finished: { order: 7, tone: "muted", label: "Finished" },
};

// Only Waiting changes with the queue mode: a paused or draining queue is why
// nothing starts. A running epic keeps its label.
export function epicBadge(state: EpicState, mode?: QueueMode): string {
  if (state === "waiting" && (mode === "paused" || mode === "draining")) {
    return `Waiting: queue ${mode}`;
  }
  return STATES[state].label;
}

const shortId = (address: string) => address.slice(address.lastIndexOf("/") + 1);

export const REPAIR_HINT = "Fix it in the TUI";

function parkedLine(t: SummaryTicket, queue: string[]): TicketLineView {
  const base = { address: t.address, id: shortId(t.address), title: t.title };
  if (t.status === "needs-repair") {
    return { ...base, tone: "parked", detail: t.reason, hint: REPAIR_HINT };
  }
  if (t.answered) {
    // The agent is gone; the ticket is in the queue for a slot.
    const place = queue.indexOf(t.address) + 1;
    return { ...base, tone: "waiting", state: place > 0 ? `Answered · #${place} in queue` : "Answered" };
  }
  if (t.answerable) {
    return { ...base, tone: "parked", detail: t.summary ?? t.reason, href: `#/t/${t.address}` };
  }
  return { ...base, tone: "parked", detail: t.reason };
}

function activeLine(t: SummaryTicket): TicketLineView {
  const base = { address: t.address, id: shortId(t.address), title: t.title };
  if (t.needs_approval) {
    return { ...base, tone: "parked", state: "needs approval", href: `#/t/${t.address}`, linkLabel: "Review →" };
  }
  return { ...base, tone: "muted", live: true };
}

function ticketLines(epic: EpicSummary, queue: string[]): TicketLineView[] {
  return [
    ...epic.active.map(activeLine),
    ...epic.parked.map((t) => parkedLine(t, queue)),
  ];
}

export function epicView(epic: EpicSummary, mode?: QueueMode, queue: string[] = []): EpicView {
  return {
    address: epic.address,
    name: epic.name,
    project: epic.project,
    state: epic.state,
    tone: STATES[epic.state].tone,
    badge: epicBadge(epic.state, mode),
    live: epic.state === "running" || epic.state === "merging",
    done: epic.done,
    total: epic.total,
    reason: epic.reason,
    tickets: ticketLines(epic, queue),
  };
}

export function sortEpics(epics: EpicSummary[]): EpicSummary[] {
  return [...epics].sort((a, b) => STATES[a.state].order - STATES[b.state].order);
}

const UNITS: [string, number][] = [
  ["d", 86_400_000],
  ["h", 3_600_000],
  ["m", 60_000],
];

export function ago(iso: string, now: Date): string {
  const ms = now.getTime() - new Date(iso).getTime();
  for (const [unit, size] of UNITS) {
    if (ms >= size) return `${Math.floor(ms / size)}${unit} ago`;
  }
  return "just now";
}

export function finishedView(f: FinishedEpic, now: Date): FinishedView {
  const when = ago(f.completed_at, now);
  return {
    address: f.address,
    name: f.name,
    project: f.project,
    done: f.done,
    total: f.total,
    oneOff: f.one_off,
    finished: when === "just now" ? "finished just now" : `finished ${when}`,
  };
}

export function snapshotView(snap: Snapshot, now: Date): SnapshotView {
  const queue = snap.pending.map((p) => p.address);
  return {
    epics: sortEpics(snap.epics).map((e) => epicView(e, snap.mode, queue)),
    recentlyFinished: snap.recently_finished.map((f) => finishedView(f, now)),
  };
}
