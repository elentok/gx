import {
  CardKindDesign,
  CardKindImplementation,
  ProgressDone,
  SectionFinished,
  SectionOnHold,
  type ArchivedEpicRow,
  type EpicListRow,
  type SessionInfo,
} from "../generated/server"
import { canManage, isExited } from "../agents/sessionWrites"

export type SelectAction = "move" | "hold" | "unhold" | "stop" | "remove" | "archive" | "unarchive"

export const SELECT_ACTIONS: SelectAction[] = [
  "move",
  "hold",
  "unhold",
  "stop",
  "remove",
  "archive",
  "unarchive",
]

/** One row of a select area, as the fit rules see it. The components build these from their own row data. */
export interface SelectRow {
  /** A row's address, or the session id of an agent row. */
  key: string
  kind: "epic" | "design" | "chat" | "agent" | "archived"
  /** False while the row is in a collapsed section. */
  visible: boolean
  /** False on a Queue agent row. */
  selectable: boolean
  /** The section a section checkbox covers: "on-hold" or "other"; absent on other rows. */
  group?: string
  queued?: boolean
  held?: boolean
  /** An epic card in Finished that is a loop epic or a Design at its last step. */
  archivable?: boolean
  session?: SessionInfo
}

export const GROUP_ON_HOLD = "on-hold"
export const GROUP_OTHER = "other"

export function isArchivable(card: EpicListRow): boolean {
  const design = card.card_kind === CardKindDesign
  const loop = design || card.card_kind === CardKindImplementation
  return card.section === SectionFinished && (design ? card.progress_step === ProgressDone : loop)
}

export function rowOfCard(card: EpicListRow, visible = true): SelectRow {
  const design = card.card_kind === CardKindDesign
  const loop = design || card.card_kind === CardKindImplementation
  return {
    key: card.address,
    kind: design ? "design" : "epic",
    visible,
    selectable: loop,
    group: card.section === SectionOnHold ? GROUP_ON_HOLD : undefined,
    queued: card.queued,
    held: card.section === SectionOnHold,
    archivable: isArchivable(card),
  }
}

export function rowOfArchived(row: ArchivedEpicRow): SelectRow {
  return { key: row.address, kind: "archived", visible: true, selectable: true }
}

export function rowOfAgent(s: SessionInfo, visible = true, group?: string): SelectRow {
  return { key: s.id, kind: "agent", visible, selectable: canManage(s), group, session: s }
}

export interface Fit {
  /** Keys of the selected rows the action fits. */
  fits: string[]
  /** One entry per skipped selected row. */
  skipped: { key: string; reason: string }[]
}

type Verdict = string | undefined

/** undefined fits; a string is the reason the row is skipped. */
const RULES: Record<SelectAction, (r: SelectRow) => Verdict> = {
  move: (r) =>
    r.kind === "epic" || r.kind === "design" || r.kind === "chat" ? undefined : "can't be moved",
  hold: (r) => {
    if (r.kind !== "epic" && r.kind !== "design") return "can't be held"
    if (r.queued) return "queued"
    return r.held ? "already on hold" : undefined
  },
  unhold: (r) => (r.held ? undefined : "not on hold"),
  stop: (r) => {
    if (!r.session) return "not an agent"
    if (!canManage(r.session)) return "managed by the queue"
    return isExited(r.session) ? "not live" : undefined
  },
  remove: (r) => {
    if (!r.session) return "not an agent"
    return canManage(r.session) ? undefined : "managed by the queue"
  },
  archive: (r) =>
    r.kind === "epic" || r.kind === "design"
      ? r.archivable
        ? undefined
        : "unfinished"
      : "can't be archived",
  unarchive: (r) => (r.kind === "archived" ? undefined : "not archived"),
}

export interface SelectState {
  on: boolean
  keys: string[]
}

export const initialSelect: SelectState = { on: false, keys: [] }

export const enter = (_: SelectState): SelectState => ({ on: true, keys: [] })
/** Leaving clears the selection. */
export const leave = (_: SelectState): SelectState => initialSelect

export function toggle(s: SelectState, key: string): SelectState {
  if (!s.on) return s
  return { ...s, keys: s.keys.includes(key) ? s.keys.filter((k) => k !== key) : [...s.keys, key] }
}

const pickable = (rows: SelectRow[]) => rows.filter((r) => r.visible && r.selectable)

/** Select all: every visible, selectable row of the area. */
export function selectAll(s: SelectState, rows: SelectRow[]): SelectState {
  return s.on ? { ...s, keys: pickable(rows).map((r) => r.key) } : s
}

export const selectNone = (s: SelectState): SelectState => (s.on ? { ...s, keys: [] } : s)

/** Drops the keys whose row left the list. A run that leaves nothing selected leaves select mode. */
export function prune(s: SelectState, rows: SelectRow[], afterRun = false): SelectState {
  const live = new Set(rows.map((r) => r.key))
  const keys = s.keys.filter((k) => live.has(k))
  if (afterRun && s.on && keys.length === 0) return initialSelect
  return keys.length === s.keys.length ? s : { ...s, keys }
}

export type GroupState = "none" | "some" | "all"

/** A section's selectable rows, hidden ones in a collapsed section included. */
const groupRows = (rows: SelectRow[], group: string) =>
  rows.filter((r) => r.selectable && r.group === group)

/** The section checkbox: all picked, none, or mixed (some). Unselectable rows do not count; collapsed rows do. */
export function groupState(s: SelectState, rows: SelectRow[], group: string): GroupState {
  const mine = groupRows(rows, group)
  const n = mine.filter((r) => s.keys.includes(r.key)).length
  if (n === 0) return "none"
  return n === mine.length ? "all" : "some"
}

/** Clicking a section checkbox selects all its rows, or clears them when all are already picked. */
export function toggleGroup(s: SelectState, rows: SelectRow[], group: string): SelectState {
  if (!s.on) return s
  const mine = groupRows(rows, group).map((r) => r.key)
  if (groupState(s, rows, group) === "all")
    return { ...s, keys: s.keys.filter((k) => !mine.includes(k)) }
  return { ...s, keys: [...s.keys, ...mine.filter((k) => !s.keys.includes(k))] }
}

export function fit(action: SelectAction, s: SelectState, rows: SelectRow[]): Fit {
  const out: Fit = { fits: [], skipped: [] }
  for (const key of s.keys) {
    const row = rows.find((r) => r.key === key)
    if (!row) continue
    const reason = RULES[action](row)
    if (reason === undefined) out.fits.push(key)
    else out.skipped.push({ key, reason })
  }
  return out
}

const NAMES: Record<SelectAction, string> = {
  move: "Move",
  hold: "Hold",
  unhold: "Take off hold",
  stop: "Stop",
  remove: "Remove",
  archive: "Archive",
  unarchive: "Unarchive",
}

/** "Archive 2 of 6", or "Archive 2" when every selected row fits. */
export function countLabel(action: SelectAction, f: Fit): string {
  const n = f.fits.length
  const m = n + f.skipped.length
  return n === m ? `${NAMES[action]} ${n}` : `${NAMES[action]} ${n} of ${m}`
}

/** The tooltip lines: one "name: reason" per skipped row. */
export function skipLines(f: Fit, nameOf: (key: string) => string = (k) => k): string[] {
  return f.skipped.map((x) => `${nameOf(x.key)}: ${x.reason}`)
}

export interface ActionView {
  action: SelectAction
  label: string
  fit: Fit
}

/** The actions to show: each with its label, and none that fits no selected row. */
export function visibleActions(s: SelectState, rows: SelectRow[]): ActionView[] {
  return SELECT_ACTIONS.map((action) => {
    const f = fit(action, s, rows)
    return { action, label: countLabel(action, f), fit: f }
  }).filter((a) => a.fit.fits.length > 0)
}
