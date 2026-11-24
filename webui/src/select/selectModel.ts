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
import { canManage } from "../agents/sessionWrites"
import { agentKindOf, type Entity } from "../actions/entity"
import { bulkBar, bulkDecl, bulkFit, bulkLabel, type BulkId } from "../actions/registry"

/** A registry action id: the bar's buttons are the registry actions that declare a bulk rule. */
export type SelectAction = BulkId

/** One row of a select area, as the fit rules see it. The components build these from their own row data. */
export interface SelectRow {
  /** A row's address, or the session id of an agent row. */
  key: string
  kind: "epic" | "design" | "chat" | "draft" | "chat-draft" | "agent" | "archived"
  /** A draft row's id (design or chat draft), which the draft Remove write takes; its `key` is the workspace membership key. */
  draftId?: string
  /** A Design's live sessions: their count is its Stop rule, and the one session is what Stop ends. */
  liveSessions?: SessionInfo[]
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

/** The registry entity of a row, which the registry's bulk rules read. */
export function entityOfRow(r: SelectRow, name = r.key): Entity {
  const base = { key: r.key, name, held: r.held, queued: r.queued }
  switch (r.kind) {
    case "epic":
      return { ...base, kind: "epic" }
    case "archived":
      return { ...base, kind: "epic", archived: true }
    case "design":
      return { ...base, kind: "design", liveSessions: r.liveSessions?.length }
    case "draft":
      return { ...base, kind: "design-draft" }
    case "chat-draft":
      return { ...base, kind: "chat-draft" }
    case "chat":
      return {
        ...base,
        kind: "chat",
        session: r.session,
        agent: r.session ? agentKindOf(r.session) : "chat",
      }
    case "agent":
      return {
        ...base,
        kind: "chat",
        session: r.session,
        agent: r.session?.queue ? "queue" : "managed",
      }
  }
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

/** The entities of the selected rows, in selection order; keys whose row left the list are dropped. */
export function selectedEntities(
  s: SelectState,
  rows: SelectRow[],
  nameOf: (key: string) => string = (k) => k,
): Entity[] {
  return s.keys.flatMap((key) => {
    const row = rows.find((r) => r.key === key)
    return row ? [entityOfRow(row, nameOf(key))] : []
  })
}

export function fitOf(action: SelectAction, s: SelectState, rows: SelectRow[]): Fit {
  return bulkFit(bulkDecl(action), selectedEntities(s, rows))
}

/** "Archive 2 of 6", or "Archive 2" when every selected row fits. */
export function countLabel(action: SelectAction, f: Fit): string {
  return bulkLabel(bulkDecl(action), f)
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

/** The actions to show, in registry order: each with its label, and none that fits no selected row. */
export function visibleActions(s: SelectState, rows: SelectRow[]): ActionView[] {
  return bulkBar(selectedEntities(s, rows)).map((v) => ({
    action: v.id,
    label: v.label,
    fit: v.fit,
  }))
}
