import { useRef } from "react"
import {
  CardKindDesign,
  SectionOnHold,
  type DesignDraftRow,
  type EpicListRow,
  type SessionInfo,
  type Workspace,
} from "../generated/server"
import { agentKindOf, type AgentKind, type Entity, type EntityKind } from "../actions/entity"
import { useActions, type ActionContext, type ReadyAction } from "../actions/useActions"
import type { SurfaceContext } from "../actions/confirmRequest"
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { defaultWriters, type ItemWriters } from "./itemWriters"

export {
  defaultWriters,
  postArchive,
  postHold,
  postMove,
  postUnhold,
  type ItemWriteResult,
  type ItemWriters,
} from "./itemWriters"

/** What the menu needs of a Design or epic card, a chat or a jump row. One-off, Follow-ups and Other get no menu. */
export interface ItemRef {
  /** The Design or epic address, the chat's session id, or the draft's workspace membership key. */
  key: string
  name: string
  kind: "design" | "epic" | "chat" | "draft" | "chat-draft"
  /** The workspace the item sits in. */
  workspace: string
  /** The epic (or the Design's -impl) holds a queue item. */
  queued?: boolean
  held?: boolean
  /** A chat's session, for Stop and Remove. */
  session?: SessionInfo
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
    session: item.session,
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

/** Actions a card's own page or row builds by hand, so the shared set leaves them out. */
const BUILT_BY_PAGE = ["open-design-epic", "open-pr", "new-session", "mark-merged", "delete-design"]
const MODAL: SurfaceContext = { mode: "modal" }

/**
 * The page-header form of {@link useItemActions}: no sections while the card is unknown. EpicPage
 * and SessionControls build the lifecycle items above by hand, so they are filtered out by id.
 */
export function useCardActions(card: EpicListRow | undefined, workspaces: Workspace[]) {
  return useItemActions(itemOfCard(card), workspaces, undefined, undefined, {
    hide: BUILT_BY_PAGE,
  })
}

function entryOf(a: ReadyAction): MenuEntry {
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
    onSelect: a.run,
  }
}

/** The registry's actions for the item, as menu groups in registry order. No item, no groups. */
export function useItemActions(
  item: ItemRef | undefined,
  workspaces: Workspace[],
  writers: ItemWriters = defaultWriters,
  /** Runs after a chat's or draft's Remove lands. */
  onRemoved?: () => void,
  opts: {
    /** Where a confirm anchors, resolved when the action runs; absent means a modal. */
    anchor?: () => HTMLElement | null | undefined
    /** Action ids the caller builds itself. */
    hide?: string[]
  } = {},
) {
  return useEntityActions(item && entityOfItem(item), workspaces, opts, {
    writers,
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
  } = {},
  ctx: Partial<ActionContext> = {},
) {
  const { actions, get } = useActions(entity, {
    surface: () => {
      const anchor = opts.anchor?.()
      return anchor ? { mode: "anchor", anchor } : MODAL
    },
    workspaces,
    promote: { connected: true },
    ...ctx,
  })
  const hide = new Set(opts.hide ?? BUILT_BY_PAGE)
  const groups: MenuEntry[][] = []
  let last: string | undefined
  for (const a of actions) {
    if (hide.has(a.id) || (a.id === "move" && workspaces.length === 0)) continue
    // A design or epic row offers no Stop; its sessions stop through Hold and Archive.
    if (a.id === "stop" && (entity?.kind === "design" || entity?.kind === "epic")) continue
    if (a.group !== last) groups.push([])
    last = a.group
    groups[groups.length - 1]!.push(entryOf(a))
  }
  return { groups, get }
}

/** The "..." menu of one item; renders nothing when it has no actions. */
export function ItemMenu({
  item,
  workspaces,
  writers,
  onRemoved,
  extra,
  lead,
  roving,
  modal,
}: {
  /** A group the caller builds by hand, above the registry's (a nested agent's Agent block). */
  lead?: MenuEntry[]
  /** True in an area with roving focus: the trigger leaves the Tab order and `.` opens the menu. */
  roving?: boolean
  item: ItemRef
  workspaces: Workspace[]
  writers?: ItemWriters
  onRemoved?: () => void
  /** A card's own entries, as one more group after the item's. */
  extra?: MenuEntry[]
  /** Confirms open as a modal, not a popover on the button (the Jump palette). */
  modal?: boolean
}) {
  const wrap = useRef<HTMLSpanElement>(null)
  // The wrapper owns the trigger, so the button is found from it when the pick runs.
  const { groups } = useItemActions(item, workspaces, writers, onRemoved, {
    anchor: modal ? undefined : () => wrap.current?.querySelector("button"),
  })
  const all = [...(lead?.length ? [lead] : []), ...groups, ...(extra?.length ? [extra] : [])]
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
