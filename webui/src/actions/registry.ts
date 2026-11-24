import { ActionStartOver, HoldQueuedText } from "../generated/server"
import { isExited } from "../agents/sessionWrites"
import {
  agentOf,
  isChatEntity,
  isDraftEntity,
  isLoopEntity,
  isPlainChat,
  type Entity,
} from "./entity"

export type Group = "open" | "edit" | "lifecycle" | "info" | "danger"

/** Menu order of the groups; Danger is the danger color. */
export const GROUP_ORDER: Group[] = ["open", "edit", "lifecycle", "info", "danger"]

/** What a confirm body returns: lines of consequence, and the input it collects (Start over's reason). */
export interface ConfirmContent {
  lines: string[]
  input?: { label: string; required?: boolean }
}

/** The confirm of one action. The rule is data; `prepare` and `body` are filled by the writers in 02. */
export interface ConfirmDecl<P = unknown> {
  /** When the confirm applies, from the entity snapshot alone. */
  rule: (e: Entity) => boolean
  /** Runs before the confirm opens (the Archive dry run). */
  prepare?: (e: Entity) => Promise<P>
  body?: (e: Entity, prepared: P) => ConfirmContent
}

export type BulkRule = (e: Entity) => string | undefined

interface Base {
  id: string
  label: string
  danger?: boolean
  /** True when the action exists for this entity in this state. */
  available: (e: Entity) => boolean
  /** Set when an available action is shown but cannot run; the text is the reason. */
  disabled?: (e: Entity) => string | undefined
  confirm: ConfirmDecl
  /** Present only on the bar's actions. Returns nothing when the row fits, else the skip reason. */
  bulk?: BulkRule
  chord?: string
}

export type ActionDecl = Base &
  ({ menu?: true; group: Group; order: number } | { menu: false; group?: never; order?: never })

const always = () => true
const never = () => false

const QUEUE_REASON = "managed by the queue"
const isQueueAgent = (e: Entity) => isChatEntity(e) && agentOf(e) === "queue"
const liveOf = (e: Entity) => e.liveSessions ?? 0

/** The page's cancel rule: a One-off that has not ended. */
export const CANCELLABLE_STATUSES = ["open", "claimed", "needs-answer", "needs-repair"] // oldword:ok
export const canCancelRun = (e: Entity) =>
  e.kind === "ticket" && !!e.oneOff && CANCELLABLE_STATUSES.includes(e.status ?? "")

const stopsOnDesign = (e: Entity) =>
  e.surface === "page" ? !!e.session && !isExited(e.session) : liveOf(e) === 1

const canStop = (e: Entity) => {
  if (e.kind === "design") return stopsOnDesign(e)
  if (!isChatEntity(e) || agentOf(e) === "queue") return false
  return !(e.session && isExited(e.session))
}

const live = (e: Entity) => isLoopEntity(e) && !e.archived
const epicOrDesign = (e: Entity) => live(e)

/** A bulk rule that skips the Queue agent first and passes every other row to `rule`. */
const bulkRule =
  (rule: BulkRule): BulkRule =>
  (e) =>
    isQueueAgent(e) ? QUEUE_REASON : rule(e)

const confirmWhen = (rule: (e: Entity) => boolean): ConfirmDecl => ({ rule })
const confirmAlways = confirmWhen(always)
const confirmNever = confirmWhen(never)

export const ACTIONS: ActionDecl[] = [
  {
    id: "open-design-epic",
    label: "Open design epic",
    group: "open",
    order: 1,
    available: (e) => e.kind === "design",
    confirm: confirmNever,
  },
  {
    id: "open-pr",
    label: "Open PR",
    group: "open",
    order: 2,
    available: (e) => e.kind === "epic" && !e.archived && !!e.prUrl,
    confirm: confirmNever,
  },
  {
    id: "rename",
    label: "Rename",
    group: "edit",
    order: 1,
    chord: "r",
    available: (e) => isPlainChat(e) || isDraftEntity(e),
    confirm: confirmNever,
  },
  {
    id: "move",
    label: "Move to workspace",
    group: "edit",
    order: 2,
    chord: "w",
    available: (e) => live(e) || isPlainChat(e) || isDraftEntity(e),
    confirm: confirmNever,
    bulk: bulkRule((e) =>
      live(e) || isPlainChat(e) || isDraftEntity(e) ? undefined : "can't be moved",
    ),
  },
  {
    id: "switch-model",
    label: "Switch model",
    group: "edit",
    order: 3,
    chord: "m",
    available: isPlainChat,
    confirm: confirmNever,
  },
  {
    id: "promote",
    label: "Promote to Design",
    group: "edit",
    order: 4,
    available: isPlainChat,
    confirm: confirmNever,
  },
  {
    id: "new-session",
    label: "New session",
    group: "lifecycle",
    order: 1,
    available: (e) => e.kind === "design",
    confirm: confirmWhen((e) => liveOf(e) > 0),
  },
  {
    id: "hold",
    label: "Hold",
    group: "lifecycle",
    order: 2,
    available: (e) => epicOrDesign(e) && !e.held,
    disabled: (e) => (e.queued ? HoldQueuedText : undefined),
    confirm: confirmWhen((e) => liveOf(e) > 0),
    bulk: bulkRule((e) => {
      if (!live(e)) return "can't be held"
      if (e.queued) return "queued"
      return e.held ? "already on hold" : undefined
    }),
  },
  {
    id: "unhold",
    label: "Take off hold",
    group: "lifecycle",
    order: 3,
    available: (e) => epicOrDesign(e) && !!e.held,
    confirm: confirmNever,
    bulk: bulkRule((e) => (e.held ? undefined : "not on hold")),
  },
  {
    id: "stop",
    label: "Stop",
    group: "lifecycle",
    order: 4,
    available: canStop,
    confirm: confirmAlways,
    bulk: bulkRule((e) => {
      if (e.kind === "design") {
        const n = liveOf(e)
        if (n === 1) return undefined
        return n === 0 ? "not live" : `${n} live sessions`
      }
      if (!isChatEntity(e)) return "not an agent"
      return e.session && isExited(e.session) ? "not live" : undefined
    }),
  },
  {
    id: "mark-merged",
    label: "Mark as merged",
    group: "lifecycle",
    order: 5,
    available: (e) => e.kind === "epic" && !e.archived,
    confirm: confirmAlways,
  },
  {
    id: "archive",
    label: "Archive",
    group: "lifecycle",
    order: 6,
    chord: "a",
    available: epicOrDesign,
    confirm: confirmAlways,
    // Like the row menu, the server's dry run decides which epics are finished enough.
    bulk: bulkRule((e) => (live(e) ? undefined : "can't be archived")),
  },
  {
    id: "unarchive",
    label: "Unarchive",
    group: "lifecycle",
    order: 7,
    chord: "a",
    available: (e) => e.kind === "epic" && !!e.archived,
    confirm: confirmNever,
    bulk: bulkRule((e) => (e.kind === "epic" && e.archived ? undefined : "not archived")),
  },
  {
    id: "copy-id",
    label: "Copy ID",
    group: "info",
    order: 1,
    chord: "y",
    available: (e) => !isDraftEntity(e),
    confirm: confirmNever,
  },
  {
    id: "start-over",
    label: "Start over",
    group: "danger",
    order: 1,
    danger: true,
    available: (e) => e.kind === "ticket" && !!e.serverActions?.includes(ActionStartOver),
    confirm: confirmAlways,
  },
  {
    id: "cancel-run",
    label: "Cancel run",
    group: "danger",
    order: 2,
    danger: true,
    available: canCancelRun,
    confirm: confirmAlways,
  },
  {
    id: "remove",
    label: "Remove",
    group: "danger",
    order: 3,
    danger: true,
    chord: "x",
    available: (e) => (isChatEntity(e) && !isQueueAgent(e)) || isDraftEntity(e),
    // An unreadable draft (emptiness unknown) is asked about.
    confirm: confirmWhen((e) => (isDraftEntity(e) ? e.draftEmpty !== true : true)),
    bulk: bulkRule((e) => (isChatEntity(e) || isDraftEntity(e) ? undefined : "not an agent")),
  },
  {
    id: "delete-design",
    label: "Delete design",
    group: "danger",
    order: 4,
    danger: true,
    available: (e) => e.kind === "design",
    confirm: confirmAlways,
  },
  // Footer-only: no group, no order, in no menu.
  { id: "queue", label: "Queue", menu: false, available: never, confirm: confirmNever },
  { id: "merge", label: "Merge", menu: false, available: never, confirm: confirmNever },
  { id: "create-pr", label: "Create PR", menu: false, available: never, confirm: confirmNever },
  {
    id: "start-design-session",
    label: "Start design session",
    menu: false,
    available: never,
    confirm: confirmNever,
  },
]

/** The chord letters of the entity actions, one per action; the pair Archive and Unarchive shares `a`. */
export const chordLetters = (actions: ActionDecl[] = ACTIONS): string[] => [
  ...new Set(actions.flatMap((a) => (a.chord ? [a.chord] : []))),
]

const rank = (a: ActionDecl) =>
  a.menu === false ? -1 : GROUP_ORDER.indexOf(a.group) * 1000 + a.order

/** Every menu action available for the entity, by group then order. Footer-only actions are left out. */
export function menuActions(e: Entity, actions: ActionDecl[] = ACTIONS): ActionDecl[] {
  return actions.filter((a) => a.menu !== false && a.available(e)).sort((a, b) => rank(a) - rank(b))
}

export const actionById = (id: string, actions: ActionDecl[] = ACTIONS) =>
  actions.find((a) => a.id === id)

export interface BulkFit {
  fits: string[]
  skipped: { key: string; reason: string }[]
}

/** Which of the selected entities the action fits. A ticket is never selectable, so it never fits. */
export function bulkFit(a: ActionDecl, selected: Entity[]): BulkFit {
  const out: BulkFit = { fits: [], skipped: [] }
  for (const e of selected) {
    const reason = e.kind === "ticket" ? "can't be selected" : a.bulk?.(e)
    if (reason === undefined) out.fits.push(e.key)
    else out.skipped.push({ key: e.key, reason })
  }
  return out
}

export interface BulkView {
  id: string
  /** "Hold 2 of 5", or "Hold 2" when every selected entity fits. */
  label: string
  fit: BulkFit
}

/** The bar's actions in registry order, each with a count label, and none that fits no selected entity. */
export function bulkBar(selected: Entity[], actions: ActionDecl[] = ACTIONS): BulkView[] {
  return actions
    .filter((a) => a.bulk)
    .sort((a, b) => rank(a) - rank(b))
    .map((a) => {
      const fit = bulkFit(a, selected)
      const n = fit.fits.length
      const m = n + fit.skipped.length
      return { id: a.id, label: n === m ? `${a.label} ${n}` : `${a.label} ${n} of ${m}`, fit }
    })
    .filter((v) => v.fit.fits.length > 0)
}

/** A bulk run confirms when any row it runs on needs it. */
export const bulkConfirms = (a: ActionDecl, selected: Entity[]): boolean =>
  selected.some((e) => a.confirm.rule(e))
