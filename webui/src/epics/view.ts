import type { EpicListRow } from "../generated/server";
import type { Tone } from "../ui/tone";
import { ago } from "../view";

export const FINISHED_CAP = 10;

export interface Section {
  key: "ready" | "designing" | "waiting" | "finished";
  title: string;
  tone: Tone;
  rows: EpicListRow[];
}

// Missing timestamps sort last.
const byTimeDesc = (time: (r: EpicListRow) => string | undefined) => (a: EpicListRow, b: EpicListRow) =>
  (time(b) ?? "").localeCompare(time(a) ?? "");
const byName = (a: EpicListRow, b: EpicListRow) => a.name.localeCompare(b.name);

const finishedAt = (r: EpicListRow) => r.merged_at ?? r.completed_at;

// The server computes each state, so grouping is a filter and a sort per section.
export function sectionsOf(epics: EpicListRow[]): Section[] {
  const inState = (state: string) => epics.filter((e) => e.state === state);
  const sections: Section[] = [
    { key: "ready", title: "Ready to queue", tone: "running", rows: inState("ready").sort(byTimeDesc((r) => r.created_at)) },
    {
      key: "designing",
      title: "Designing",
      tone: "muted",
      rows: inState("designing").sort(byTimeDesc((r) => r.last_activity_at)),
    },
    { key: "waiting", title: "Waiting to design", tone: "muted", rows: inState("waiting-to-design").sort(byName) },
    {
      key: "finished",
      title: "Recently finished",
      tone: "done",
      rows: inState("finished").sort(byTimeDesc(finishedAt)).slice(0, FINISHED_CAP),
    },
  ];
  return sections.filter((s) => s.rows.length > 0);
}

export const searchEpics = (epics: EpicListRow[], query: string): EpicListRow[] => {
  const q = query.trim().toLowerCase();
  return epics.filter((e) => e.name.toLowerCase().includes(q)).sort(byName);
};

const STATE_BADGES: Record<string, { label: string; tone: Tone }> = {
  ready: { label: "Ready to queue", tone: "running" },
  designing: { label: "Designing", tone: "muted" },
  "waiting-to-design": { label: "Waiting to design", tone: "muted" },
  finished: { label: "Finished", tone: "done" },
  queued: { label: "Queued", tone: "running" },
  running: { label: "Running", tone: "running" },
  "awaiting-merge": { label: "Awaiting merge", tone: "waiting" },
  parked: { label: "Parked", tone: "parked" },
  deadlocked: { label: "Deadlocked", tone: "deadlocked" },
  draft: { label: "Draft", tone: "muted" },
  "no-open-tickets": { label: "No open tickets", tone: "muted" },
};

export const stateBadge = (state: string) => STATE_BADGES[state] ?? { label: state, tone: "muted" as Tone };

// The time wording follows the section the row sits in; search rows follow the epic's state.
export function timePart(row: EpicListRow, now: Date): string | undefined {
  const [word, iso] =
    row.state === "finished"
      ? ["finished", finishedAt(row)]
      : row.state === "designing"
        ? ["active", row.last_activity_at]
        : ["created", row.created_at];
  return iso ? `${word} ${ago(iso, now)}` : undefined;
}
