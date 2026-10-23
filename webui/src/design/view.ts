import type { EpicListRow, PickerSection, SessionInfo } from "../generated/server";
import { designHref, epicHref } from "../hrefs";

// The generated file is imported for types only: its tygo constants are not runtime values.
export const IN_PROGRESS: PickerSection = "in-progress";
const READY_TO_QUEUE: PickerSection = "ready-to-queue";

export interface PickerRow {
  row: EpicListRow;
  /** A session of the Design is running. */
  live: boolean;
  /** The session waits on a Grilling round. */
  question: boolean;
  /** The session failed for a reason other than a rate limit. */
  failed: boolean;
}

export interface PickerGroup {
  key: PickerSection;
  title: string;
  rows: PickerRow[];
}

const SECTIONS: { key: PickerSection; title: string }[] = [
  { key: IN_PROGRESS, title: "In progress" },
  { key: "not-started", title: "Not started" },
  { key: READY_TO_QUEUE, title: "Ready to queue" },
  { key: "recently-finished", title: "Recently finished" },
];

// The server raises a Grilling round's Actionable with this reason prefix (server/sessionactionable.go).
const GRILLING_REASON_PREFIX = "Grilling";

/** A Design's session sits on a ticket of its map epic, so the epic is the ticket address before the slash. */
const designOf = (s: SessionInfo) => (s.design && s.ticket ? s.ticket.slice(0, s.ticket.indexOf("/")) : undefined);

function pickerRow(row: EpicListRow, sessions: SessionInfo[]): PickerRow {
  const mine = sessions.filter((s) => designOf(s) === row.address);
  return {
    row,
    live: mine.some((s) => s.status.state !== "exited"),
    question: mine.some((s) => s.actionable.reason?.startsWith(GRILLING_REASON_PREFIX)),
    failed: mine.some((s) => s.failure !== undefined && s.failure.kind !== "rate-limit"),
  };
}

const needsYou = (r: PickerRow) => r.question || r.failed;
const sortAt = (r: PickerRow) => Date.parse(r.row.picker_sort_at ?? "") || 0;

/** Waiting or failed Designs first, then live ones, then the rest newest first. */
export const compareRows = (a: PickerRow, b: PickerRow) =>
  Number(needsYou(b)) - Number(needsYou(a)) || Number(b.live) - Number(a.live) || sortAt(b) - sortAt(a);

/** The non-empty sections of the Designs picker. */
export function pickerGroups(epics: EpicListRow[], sessions: SessionInfo[]): PickerGroup[] {
  const maps = epics.filter((e) => e.kind === "map");
  return SECTIONS.map(({ key, title }) => ({
    key,
    title,
    rows: maps
      .filter((e) => e.picker_section === key)
      .map((e) => pickerRow(e, sessions))
      .sort(compareRows),
  })).filter((g) => g.rows.length > 0);
}

/** Keeps the rows whose slug or name contains the query, and drops sections left empty. */
export function filterGroups(groups: PickerGroup[], query: string): PickerGroup[] {
  const q = query.trim().toLowerCase();
  if (!q) return groups;
  return groups
    .map((g) => ({ ...g, rows: g.rows.filter((r) => `${r.row.address} ${r.row.name}`.toLowerCase().includes(q)) }))
    .filter((g) => g.rows.length > 0);
}

/** A Design needs you when it has a waiting question or a failure. */
export const needsYouCount = (groups: PickerGroup[], current?: string) =>
  groups.reduce((n, g) => n + g.rows.filter((r) => needsYou(r) && r.row.address !== current).length, 0);

/** A Ready to queue row opens its implementation epic; every other row opens its Design. */
export const rowHref = (section: PickerSection, r: EpicListRow) =>
  section === READY_TO_QUEUE && r.impl_address ? epicHref(r.impl_address) : designHref(r.address);
