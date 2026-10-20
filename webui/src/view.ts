import type { EpicState, EpicSummary, FinishedEpic, QueueMode, Snapshot, SummaryTicket } from "./snapshot";
import type { Tone } from "./ui/tone";

export interface TicketLineView {
  address: string;
  id: string;
  title: string;
  tone: Tone;
  detail?: string;
}

export interface EpicView {
  address: string;
  name: string;
  project: string;
  tone: Tone;
  badge: string;
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
  running: { order: 2, tone: "running", label: "Running" },
  waiting: { order: 3, tone: "waiting", label: "Waiting for a slot" },
  "awaiting-merge": { order: 4, tone: "muted", label: "Awaiting merge" },
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

function ticketLines(epic: EpicSummary): TicketLineView[] {
  const line = (t: SummaryTicket, tone: Tone, detail?: string): TicketLineView => ({
    address: t.address,
    id: shortId(t.address),
    title: t.title,
    tone,
    detail,
  });
  return [
    ...epic.active.map((t) => line(t, "muted")),
    ...epic.parked.map((t) => line(t, "parked", t.reason)),
  ];
}

export function epicView(epic: EpicSummary, mode?: QueueMode): EpicView {
  return {
    address: epic.address,
    name: epic.name,
    project: epic.project,
    tone: STATES[epic.state].tone,
    badge: epicBadge(epic.state, mode),
    done: epic.done,
    total: epic.total,
    reason: epic.reason,
    tickets: ticketLines(epic),
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
    finished: when === "just now" ? "finished just now" : `finished ${when}`,
  };
}

export function snapshotView(snap: Snapshot, now: Date): SnapshotView {
  return {
    epics: sortEpics(snap.epics).map((e) => epicView(e, snap.mode)),
    recentlyFinished: snap.recently_finished.map((f) => finishedView(f, now)),
  };
}
