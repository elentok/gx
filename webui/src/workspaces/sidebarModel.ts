import {
  CardKindDesign,
  CardKindImplementation,
  PlacementChat,
  PlacementEpic,
  PlacementOther,
  SectionNeedsYou,
  SectionOnHold,
  SessionWorking,
  SortActivity,
  SortName,
  SortNeedsYou,
  SortProject,
  SortStatus,
  type EpicListRow,
  type SessionInfo,
  type SessionPlacement,
  type Sort,
} from "../generated/server"
import { isActionable, stateLabel } from "../agents/rules"
import { RING_KIND, isLive, ringProgress } from "../epics/ring"
import { agentHref, designHref } from "../hrefs"
import { isArchivable } from "../select/selectModel"
import { titleHref } from "../epics/cardLinks"
import type { Kind, Tone } from "../ui/tone"

export const NEEDS_YOU = "Needs you"
export const WORKING = "Working"
export const MAX_NUMBERED = 9

export interface SidebarStatus {
  words: string
  /** waiting is amber, running blue, muted is the dimmed server word. */
  tone: Extract<Tone, "waiting" | "running" | "muted">
}

export interface SidebarAgent {
  id: string
  label: string
  href: string
  status: SidebarStatus
  /** A Fix agent (session with fix_of set); drawn with a wrench. */
  fix?: boolean
  /** The session this row was built from, for the agent menu. */
  session: SessionInfo
}

export interface SidebarRow {
  key: string
  /** Design is purple, epic teal (the ring kinds), chat pink. */
  kind: Kind | "chat"
  title: string
  href: string
  status: SidebarStatus
  /** Ring fill 0 to 1; absent on chats, which show the chat icon. */
  ring?: number
  /** Alt+n shortcut, 1 to 9, on own rows only. */
  number?: number
  /** The section this row falls under in the By project and By status sorts; absent in the others. */
  group?: string
  /** True on On hold rows so the view dims their words. */
  held?: boolean
  /** The card's epic (or a Design's -impl) holds a queue item; Hold is then disabled. */
  queued?: boolean
  /** A Finished loop epic or a Design at its last step: Archive fits it. */
  archivable?: boolean
  /** A chat's session, for its menu's Stop and Remove. */
  session?: SessionInfo
  agents: SidebarAgent[]
}

export interface Sidebar {
  rows: SidebarRow[]
  held: SidebarRow[]
  other: SidebarAgent[]
}

export interface SidebarInput {
  rows: EpicListRow[]
  sessions: SessionInfo[]
  placements: SessionPlacement[]
  workspace: string
  /** The active workspace's Sort value; missing or unknown reads as needs-you. */
  sort?: string
}

const isCard = (r: EpicListRow) =>
  r.card_kind === CardKindDesign || r.card_kind === CardKindImplementation

export function cardStatus(r: EpicListRow): SidebarStatus {
  if (r.section === SectionNeedsYou) return { words: NEEDS_YOU, tone: "waiting" }
  if (isLive(r)) return { words: WORKING, tone: "running" }
  return { words: r.status_words ?? "", tone: "muted" }
}

export function sessionStatus(s: SessionInfo): SidebarStatus {
  if (isActionable(s)) return { words: NEEDS_YOU, tone: "waiting" }
  if (s.status.state === SessionWorking) return { words: WORKING, tone: "running" }
  return { words: stateLabel(s), tone: "muted" }
}

/** A Design's agent works in its own session, not the queue: a working session reads as Working. */
function designStatus(r: EpicListRow, sessions: SessionInfo[]): SidebarStatus {
  const status = cardStatus(r)
  if (status.tone === "waiting" || status.tone === "running") return status
  const working = sessions.some((s) => s.status.state === SessionWorking && !s.status.background)
  return working ? { words: WORKING, tone: "running" } : status
}

const agentRow = (s: SessionInfo): SidebarAgent => ({
  id: s.id,
  label: s.name || s.ticket || s.id,
  href: agentHref(s.id),
  status: sessionStatus(s),
  fix: !!s.fix_of,
  session: s,
})

function cardRow(
  r: EpicListRow,
  agents: SidebarAgent[],
  held: boolean,
  sessions: SessionInfo[] = [],
): SidebarRow {
  const kind = RING_KIND[r.card_kind ?? CardKindImplementation]
  return {
    key: r.address,
    kind,
    title: r.name,
    href: kind === "design" ? designHref(r.address) : titleHref(r),
    status: kind === "design" ? designStatus(r, sessions) : cardStatus(r),
    ring: ringProgress(r),
    ...(held ? { held: true } : {}),
    ...(r.queued ? { queued: true } : {}),
    ...(isArchivable(r) ? { archivable: true } : {}),
    // A Design is one row: its session shows on the Design page, not nested under it.
    agents: kind === "design" ? [] : agents,
  }
}

/** A missing or unknown value reads as needs-you. */
const sortOf = (sort: string | undefined): Sort =>
  sort === SortName || sort === SortActivity || sort === SortProject || sort === SortStatus
    ? sort
    : SortNeedsYou

export const CHATS_GROUP = "Chats"

/** Needs you, then Working, then the rest by their words. */
const statusRank = (words: string) => (words === NEEDS_YOU ? 0 : words === WORKING ? 1 : 2)
const cmpStatus = (a: string, b: string) =>
  statusRank(a) - statusRank(b) || a.toLowerCase().localeCompare(b.toLowerCase())

const time = (iso: string | undefined): number | undefined => {
  const t = iso ? Date.parse(iso) : NaN
  return Number.isNaN(t) ? undefined : t
}

const newest = (times: (number | undefined)[]): number | undefined => {
  const known = times.filter((t): t is number => t !== undefined)
  return known.length ? Math.max(...known) : undefined
}

interface Sortable<T> {
  item: T
  name: string
  needsYou: boolean
  at: number | undefined
  /** The project of a card (chats: CHATS_GROUP). */
  project: string
  /** The status words. */
  words: string
}

/** Array.sort is stable, so ties keep server order. */
function sortBy<T>(sort: Sort, entries: Sortable<T>[]): T[] {
  const byName = (a: Sortable<T>, b: Sortable<T>) =>
    a.name.toLowerCase().localeCompare(b.name.toLowerCase())
  const cmp = (a: Sortable<T>, b: Sortable<T>) => {
    if (sort === SortName) return byName(a, b)
    if (sort === SortProject)
      return a.project.toLowerCase().localeCompare(b.project.toLowerCase()) || byName(a, b)
    if (sort === SortStatus) return cmpStatus(a.words, b.words) || byName(a, b)
    if (sort === SortActivity) {
      if (a.at === undefined || b.at === undefined)
        return a.at === b.at ? byName(a, b) : a.at === undefined ? 1 : -1
      return b.at - a.at || 0
    }
    return Number(b.needsYou) - Number(a.needsYou) || byName(a, b)
  }
  return [...entries].sort(cmp).map((e) => e.item)
}

export function buildSidebar({
  rows,
  sessions,
  placements,
  workspace,
  sort,
}: SidebarInput): Sidebar {
  const mode = sortOf(sort)
  const byId = new Map(sessions.map((s) => [s.id, s]))
  const placed = placements.flatMap((p) => {
    const s = byId.get(p.session)
    return s ? [{ p, s }] : []
  })
  const sessionsOf = (r: EpicListRow) =>
    placed
      .filter(
        ({ p }) =>
          p.kind === PlacementEpic &&
          !!p.epic &&
          (p.epic === r.address || !!r.addresses?.includes(p.epic)),
      )
      .map(({ s }) => s)
  const sessionAt = (s: SessionInfo) => time(s.status_changed_at)
  const sortAgents = (ss: SessionInfo[]) =>
    sortBy(
      mode,
      ss.map((s) => ({
        item: agentRow(s),
        name: s.name || s.ticket || s.id,
        needsYou: isActionable(s),
        at: sessionAt(s),
        project: "",
        words: sessionStatus(s).words,
      })),
    )
  const cardEntry = (r: EpicListRow, held: boolean): Sortable<SidebarRow> => {
    const ss = sessionsOf(r)
    const row = cardRow(r, held ? [] : sortAgents(ss), held, ss)
    return {
      item: row,
      name: r.name,
      needsYou: row.status.words === NEEDS_YOU,
      project: r.project,
      words: row.status.words,
      at: newest([time(r.last_activity_at), ...ss.map(sessionAt)]),
    }
  }

  const mine = rows.filter((r) => isCard(r) && r.workspace === workspace)
  const own = mine.filter((r) => r.section !== SectionOnHold).map((r) => cardEntry(r, false))
  const chats = placed
    .filter(({ p }) => p.kind === PlacementChat && p.workspace === workspace)
    .map(({ s }): Sortable<SidebarRow> => {
      const status = sessionStatus(s)
      return {
        item: {
          key: s.id,
          kind: "chat",
          title: s.name || s.id,
          href: agentHref(s.id),
          status,
          session: s,
          agents: [],
        },
        name: s.name || s.id,
        needsYou: status.words === NEEDS_YOU,
        project: CHATS_GROUP,
        words: status.words,
        at: sessionAt(s),
      }
    })
  const all = sortBy(mode, [...own, ...chats])
  if (mode === SortProject || mode === SortStatus) {
    const entryOf = new Map([...own, ...chats].map((e) => [e.item, e]))
    for (const row of all) {
      const e = entryOf.get(row)!
      row.group = mode === SortProject ? e.project : e.words
    }
  }
  all.forEach((row, i) => {
    if (i < MAX_NUMBERED) row.number = i + 1
  })

  return {
    rows: all,
    held: sortBy(
      mode,
      mine.filter((r) => r.section === SectionOnHold).map((r) => cardEntry(r, true)),
    ),
    other: sortAgents(placed.filter(({ p }) => p.kind === PlacementOther).map(({ s }) => s)),
  }
}
