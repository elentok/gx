import { useRef } from "react"
import {
  CardKindDesign,
  CardKindOneOff,
  SectionOnHold,
  type DesignDraftRow,
  type EpicListRow,
  type SessionInfo,
  type Workspace,
} from "../generated/server"
import { isExited } from "../agents/sessionWrites"
import { designOf } from "../design/view"
import { agentKindOf, type AgentKind, type Entity, type EntityKind } from "../actions/entity"
import {
  useActions,
  type ActionContext,
  type ActionWriters,
  type ReadyAction,
} from "../actions/useActions"
import type { SurfaceContext } from "../actions/confirmRequest"
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { defaultWriters } from "./itemWriters"

export {
  defaultWriters,
  postArchive,
  postHold,
  postMove,
  postUnhold,
  type ItemWriteResult,
  type ItemWriters,
} from "./itemWriters"

/** What the menu needs of a card, a ticket card, a chat or a jump row. */
export interface ItemRef {
  /** The Design or epic address, the chat's session id, or the draft's workspace membership key. */
  key: string
  name: string
  kind: "design" | "epic" | "chat" | "draft" | "chat-draft" | "ticket"
  /** The workspace the item sits in; a card outside every workspace has none. */
  workspace?: string
  prUrl?: string
  /** The epic (or the Design's -impl) holds a queue item. */
  queued?: boolean
  held?: boolean
  /** A chat's session, for Stop and Remove; a Design row with one live session carries that one. */
  session?: SessionInfo
  /** A Design's live session count; its Stop is offered only at exactly one. */
  liveSessions?: number
  /** A draft's row, for Rename and Remove. */
  draft?: DesignDraftRow
  /** Overrides the agent kind the session implies (a nested sidebar agent is never a plain chat). */
  agent?: AgentKind
}

const ENTITY_KIND: Record<ItemRef["kind"], EntityKind> = {
  design: "design",
  epic: "epic",
  chat: "chat",
  draft: "design-draft",
  "chat-draft": "chat-draft",
  ticket: "ticket",
}

/** The registry's view of a row. */
export function entityOfItem(item: ItemRef): Entity {
  return {
    kind: ENTITY_KIND[item.kind],
    key: item.key,
    name: item.name,
    surface: "row",
    workspace: item.workspace,
    held: item.held,
    queued: item.queued,
    prUrl: item.prUrl,
    session: item.session,
    liveSessions: item.liveSessions,
    agent: item.agent ?? (item.session ? agentKindOf(item.session) : undefined),
  }
}

/** The menu target of a Design or epic card; none for a card outside every workspace (One-off, Follow-ups, Other). */
export function itemOfCard(card: EpicListRow | undefined): ItemRef | undefined {
  if (!card?.workspace) return undefined
  return {
    key: card.address,
    name: card.name,
    kind: card.card_kind === CardKindDesign ? "design" : "epic",
    workspace: card.workspace,
    queued: card.queued,
    held: card.section === SectionOnHold,
  }
}

/**
 * The menu target of any card on the Epics page. A One-off card is a ticket; a card outside every
 * workspace is an epic with none, so the workspace actions stay off it.
 */
export function itemOfAnyCard(card: EpicListRow, sessions: SessionInfo[] = []): ItemRef {
  const base = {
    key: card.address,
    name: card.name,
    workspace: card.workspace || undefined,
    prUrl: card.pr?.url,
  }
  if (card.card_kind === CardKindOneOff) return { ...base, kind: "ticket" }
  const design = card.card_kind === CardKindDesign
  return {
    ...base,
    kind: design ? "design" : "epic",
    queued: card.queued,
    held: card.section === SectionOnHold,
    ...(design ? liveOfDesign(sessions.filter((s) => designOf(s) === card.address)) : {}),
  }
}

/** A Design's Stop data: the live count, and the session when exactly one is live. */
export function liveOfDesign(sessions: SessionInfo[]): Pick<ItemRef, "liveSessions" | "session"> {
  const live = sessions.filter((s) => !isExited(s))
  return { liveSessions: live.length, session: live.length === 1 ? live[0] : undefined }
}

/** Actions a card's own page or row builds by hand, so the shared set leaves them out. */
const BUILT_BY_PAGE = ["open-design-epic", "open-pr", "new-session", "mark-merged", "delete-design"]
const MODAL: SurfaceContext = { mode: "modal" }

/** Splits ready actions into menu groups, in registry order. */
export function groupsOf(actions: ReadyAction[]): MenuEntry[][] {
  const groups: MenuEntry[][] = []
  let last: string | undefined
  for (const a of actions) {
    if (a.group !== last) groups.push([])
    last = a.group
    groups[groups.length - 1]!.push(entryOf(a))
  }
  return groups
}

export function entryOf(a: ReadyAction): MenuEntry {
  if (a.items) {
    return {
      kind: "submenu",
      value: a.id,
      label: a.label,
      items: a.items.map((i) => ({
        value: `move-${i.label}`,
        label: i.label,
        current: i.current,
        inlineHint: true,
        hint: i.current ? "✓" : undefined,
        disabled: a.disabled,
        onSelect: i.run,
      })),
    }
  }
  return {
    value: a.id,
    label: a.label,
    danger: a.danger || undefined,
    disabled: a.disabled,
    hint: a.reason,
    onSelect: () => a.run(),
  }
}

/** The registry's actions for the item, as menu groups in registry order. No item, no groups. */
export function useItemActions(
  item: ItemRef | undefined,
  workspaces: Workspace[],
  writers: Partial<ActionWriters> = defaultWriters,
  /** Runs after a chat's or draft's Remove lands. */
  onRemoved?: () => void,
  opts: {
    /** The list state token Mark as merged posts. */
    state?: string
    /** Where a confirm anchors, resolved when the action runs; absent means a modal. */
    anchor?: () => HTMLElement | null | undefined
    /** Action ids the caller builds itself. */
    hide?: string[]
  } = {},
) {
  return useEntityActions(item && entityOfItem(item), workspaces, opts, {
    writers,
    state: opts.state,
    onRemoved,
    draft: item?.draft,
    promote: { connected: true },
  })
}

/** The menu groups and the ready actions of one entity, for a row or a page that reads the registry. */
export function useEntityActions(
  entity: Entity | undefined,
  workspaces: Workspace[],
  opts: {
    anchor?: () => HTMLElement | null | undefined
    hide?: string[]
    /** The entity key of the page the menu sits on; Rename on that page edits in place. */
    page?: string
  } = {},
  ctx: Partial<ActionContext> = {},
) {
  const { actions, get, stopSession } = useActions(entity, {
    surface: () => {
      const anchor = opts.anchor?.()
      return anchor ? { mode: "anchor", anchor, page: opts.page } : { ...MODAL, page: opts.page }
    },
    workspaces,
    promote: { connected: true },
    ...ctx,
  })
  const hide = new Set(opts.hide ?? BUILT_BY_PAGE)
  const shown = actions.filter(
    (a) => !hide.has(a.id) && !(a.id === "move" && workspaces.length === 0),
  )
  return { groups: groupsOf(shown), get, stopSession }
}

/** The "..." menu of one item; renders nothing when it has no actions. */
export function ItemMenu({
  item,
  workspaces,
  writers,
  onRemoved,
  lead,
  roving,
  modal,
  hide,
  state,
}: {
  /** A group the caller builds by hand, above the registry's (a nested agent's Agent block). */
  lead?: MenuEntry[]
  /** True in an area with roving focus: the trigger leaves the Tab order and `.` opens the menu. */
  roving?: boolean
  item: ItemRef
  workspaces: Workspace[]
  writers?: Partial<ActionWriters>
  onRemoved?: () => void
  /** Confirms open as a modal, not a popover on the button (the Jump palette). */
  modal?: boolean
  /** Action ids the caller leaves out; absent hides the ones a page builds by hand. */
  hide?: string[]
  /** The list state token Mark as merged posts. */
  state?: string
}) {
  const wrap = useRef<HTMLSpanElement>(null)
  // The wrapper owns the trigger, so the button is found from it when the pick runs.
  const { groups } = useItemActions(item, workspaces, writers, onRemoved, {
    anchor: modal ? undefined : () => wrap.current?.querySelector("button"),
    hide,
    state,
  })
  const all = [...(lead?.length ? [lead] : []), ...groups]
  if (all.length === 0) return null
  return (
    <span ref={wrap} style={{ display: "contents" }}>
      <GxMenu
        trigger={<MoreActionsButton tabIndex={roving ? -1 : undefined} />}
        items={menuGroups(all)}
      />
    </span>
  )
}
