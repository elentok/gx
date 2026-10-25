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
  /** No map yet: the row is built from the Design's session and sits at the Chart step. */
  chart?: boolean;
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

/** A Design's session sits on a ticket of its map epic, or on the epic itself before Chart makes the map. */
const designOf = (s: SessionInfo) => (s.design && s.ticket ? s.ticket.split("/")[0] : undefined);

const byNewest = (a: SessionInfo, b: SessionInfo) => Date.parse(b.status_changed_at) - Date.parse(a.status_changed_at);

const byNewestRun = (a: SessionInfo, b: SessionInfo) => Date.parse(b.run_started_at) - Date.parse(a.run_started_at);

/** A Design's live sessions, latest run start first: the order designs/open picks by (server/designs.go). */
// promptable mirrors the server's SessionStatus.Promptable: Idle, or a finished turn that left a
// background task running.
export const promptable = (s: SessionInfo) =>
  s.status.state === "idle" || (s.status.state === "working" && !!s.status.background);

export const liveSessions = (address: string, sessions: SessionInfo[]) =>
  sessions.filter((s) => designOf(s) === address && s.status.state !== "exited").sort(byNewestRun);

/** The session that drives a Design: the newest live one, else the newest Exited one. */
export const designSession = (address: string, sessions: SessionInfo[]): SessionInfo | undefined =>
  liveSessions(address, sessions)[0] ?? sessions.filter((s) => designOf(s) === address).sort(byNewest)[0];

/** Whether the Design has any session, live or Exited. Removed sessions are not in the list. */
export const hasDesignSession = (address: string, sessions: SessionInfo[]) =>
  sessions.some((s) => designOf(s) === address);

function pickerRow(row: EpicListRow, sessions: SessionInfo[]): PickerRow {
  const mine = sessions.filter((s) => designOf(s) === row.address);
  return {
    row,
    live: mine.some((s) => s.status.state !== "exited"),
    question: mine.some((s) => s.actionable.grilling),
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
  const known = new Set(epics.filter((e) => !e.map_pending).map((e) => e.address));
  return SECTIONS.map(({ key, title }) => ({
    key,
    title,
    rows: [
      ...maps.filter((e) => e.picker_section === key).map((e) => pickerRow(e, sessions)),
      ...(key === IN_PROGRESS ? sessionOnlyRows(known, sessions) : []),
    ].sort(compareRows),
  })).filter((g) => g.rows.length > 0);
}

/** Designs whose map Chart has not made yet: one row per address, from its newest session. */
function sessionOnlyRows(known: Set<string>, sessions: SessionInfo[]): PickerRow[] {
  const addresses = new Set(sessions.flatMap((s) => designOf(s) ?? []).filter((a) => !known.has(a)));
  return [...addresses].map((address) => {
    const newest = designSession(address, sessions)!;
    const row: EpicListRow = {
      address,
      project: newest.project,
      name: address.slice(address.indexOf(":") + 1),
      kind: "map",
      state: "designing",
      done: 0,
      total: 0,
      queued: false,
      queueable: false,
      blocked_by_epics: [],
      picker_section: IN_PROGRESS,
      picker_sort_at: newest.status_changed_at,
    };
    return { ...pickerRow(row, sessions), chart: true };
  });
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
