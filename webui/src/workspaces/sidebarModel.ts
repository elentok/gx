import {
  CardKindDesign,
  CardKindImplementation,
  PlacementChat,
  PlacementEpic,
  PlacementOther,
  SectionNeedsYou,
  SectionOnHold,
  SessionWorking,
  type EpicListRow,
  type SessionInfo,
  type SessionPlacement,
} from "../generated/server";
import { isActionable, stateLabel } from "../agents/rules";
import { isCurrent } from "../epics/list";
import { RING_KIND, isLive, ringProgress } from "../epics/ring";
import { agentHref } from "../hrefs";
import { titleHref } from "../epics/cardLinks";
import type { Kind, Tone } from "../ui/tone";

export const NEEDS_YOU = "Needs you";
export const WORKING = "Working";
export const MAX_NUMBERED = 9;

export interface SidebarStatus {
  words: string;
  /** waiting is amber, running blue, muted is the dimmed server word. */
  tone: Extract<Tone, "waiting" | "running" | "muted">;
}

export interface SidebarAgent {
  id: string;
  label: string;
  href: string;
  status: SidebarStatus;
}

export interface SidebarRow {
  key: string;
  /** Design is purple, epic teal (the ring kinds), chat pink. */
  kind: Kind | "chat";
  title: string;
  href: string;
  status: SidebarStatus;
  /** Ring fill 0 to 1; absent on chats, which show the chat icon. */
  ring?: number;
  /** Alt+n shortcut, 1 to 9, on own rows only. */
  number?: number;
  /** True on On hold rows so the view dims their words. */
  held?: boolean;
  /** The open page's epic starts expanded. */
  expanded?: boolean;
  agents: SidebarAgent[];
}

export interface Sidebar {
  rows: SidebarRow[];
  held: SidebarRow[];
  other: SidebarAgent[];
}

export interface SidebarInput {
  rows: EpicListRow[];
  sessions: SessionInfo[];
  placements: SessionPlacement[];
  workspace: string;
  /** The open page's epic address, or "". */
  current: string;
}

const isCard = (r: EpicListRow) => r.card_kind === CardKindDesign || r.card_kind === CardKindImplementation;

function cardStatus(r: EpicListRow): SidebarStatus {
  if (r.section === SectionNeedsYou) return { words: NEEDS_YOU, tone: "waiting" };
  if (isLive(r)) return { words: WORKING, tone: "running" };
  return { words: r.status_words ?? "", tone: "muted" };
}

function sessionStatus(s: SessionInfo): SidebarStatus {
  if (isActionable(s)) return { words: NEEDS_YOU, tone: "waiting" };
  if (s.status.state === SessionWorking) return { words: WORKING, tone: "running" };
  return { words: stateLabel(s), tone: "muted" };
}

const agentRow = (s: SessionInfo): SidebarAgent => ({
  id: s.id,
  label: s.name || s.ticket || s.id,
  href: agentHref(s.id),
  status: sessionStatus(s),
});

function cardRow(r: EpicListRow, agents: SidebarAgent[], current: string, held: boolean): SidebarRow {
  return {
    key: r.address,
    kind: RING_KIND[r.card_kind ?? CardKindImplementation],
    title: r.name,
    href: titleHref(r),
    status: cardStatus(r),
    ring: ringProgress(r),
    ...(held ? { held: true } : {}),
    ...(isCurrent(r, current) ? { expanded: true } : {}),
    agents,
  };
}

export function buildSidebar({ rows, sessions, placements, workspace, current }: SidebarInput): Sidebar {
  const byId = new Map(sessions.map((s) => [s.id, s]));
  const placed = placements.flatMap((p) => {
    const s = byId.get(p.session);
    return s ? [{ p, s }] : [];
  });
  const agentsOf = (r: EpicListRow) =>
    placed
      .filter(({ p }) => p.kind === PlacementEpic && !!p.epic && (p.epic === r.address || !!r.addresses?.includes(p.epic)))
      .map(({ s }) => agentRow(s));

  const mine = rows.filter((r) => isCard(r) && r.workspace === workspace);
  const own = mine.filter((r) => r.section !== SectionOnHold).map((r) => cardRow(r, agentsOf(r), current, false));
  const chats = placed
    .filter(({ p }) => p.kind === PlacementChat && p.workspace === workspace)
    .map(({ s }): SidebarRow => ({
      key: s.id,
      kind: "chat",
      title: s.name || s.id,
      href: agentHref(s.id),
      status: sessionStatus(s),
      agents: [],
    }));
  const all = [...own, ...chats];
  all.forEach((row, i) => {
    if (i < MAX_NUMBERED) row.number = i + 1;
  });

  return {
    rows: all,
    held: mine.filter((r) => r.section === SectionOnHold).map((r) => cardRow(r, [], current, true)),
    other: placed.filter(({ p }) => p.kind === PlacementOther).map(({ s }) => agentRow(s)),
  };
}
