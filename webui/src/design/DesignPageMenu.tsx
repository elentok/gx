import { useRef } from "react"
import { useNavigate } from "react-router"
import { openConfirm } from "../actions/confirmRequest"
import type { Entity } from "../actions/entity"
import { useConnectionContext } from "../connection"
import { SectionOnHold, type EpicListRow, type SessionInfo } from "../generated/server"
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { useWide } from "../useWide"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import { useEntityActions } from "../workspaces/itemActions"
import {
  postDeleteDesign,
  postStopSession,
  rowToken,
  type DeleteDesign,
  type OpenFlow,
  type StopSession,
} from "./open"
import { agentSection, liveSessionEntries } from "./SessionControls"

/** The writers the page injects; each wins over the default endpoint. */
export interface DesignMenuWriters {
  open?: OpenFlow
  stop?: StopSession
  remove?: DeleteDesign
}

/**
 * The Design page's "⋯" menu. The Agent block and the live-session list stay above; every other item
 * comes from the registry, and each confirm anchors on this button.
 */
export function DesignPageMenu({
  address,
  name,
  card,
  deletable,
  shown,
  live,
  onSelect,
  onOpenChange,
  writers,
}: {
  address: string
  name: string
  card?: EpicListRow
  /** The map has no Destination and no tickets yet, so it can still be deleted. */
  deletable?: boolean
  shown?: SessionInfo
  /** Live design sessions, newest first. */
  live: SessionInfo[]
  onSelect: (id: string) => void
  /** True while the menu is open; the page holds its keyboard layout then. */
  onOpenChange?: (open: boolean) => void
  writers: DesignMenuWriters
}) {
  const conn = useConnectionContext()
  const navigate = useNavigate()
  const wide = useWide()
  const wrap = useRef<HTMLSpanElement>(null)
  const { workspaces } = useWorkspaces(conn)
  const anchor = () => wrap.current?.querySelector("button")
  const entity: Entity = {
    kind: "design",
    key: address,
    name,
    surface: "page",
    workspace: card?.workspace,
    held: card?.section === SectionOnHold,
    queued: card?.queued,
    session: shown,
    liveSessions: live.length,
  }
  const hide = [
    // The Start button begins the first session; New session needs one to replace.
    ...(shown ? [] : ["new-session"]),
    ...(deletable && live.length === 0 ? [] : ["delete-design"]),
  ]
  const { groups: registry } = useEntityActions(
    entity,
    workspaces,
    { hide, anchor },
    {
      writers: {
        navigate: (href) => navigate(href.replace(/^#/, "")),
        // An absent writer must stay absent: an undefined value would hide the default.
        ...(writers.open && { openDesign: writers.open }),
        ...(writers.stop && { stop: writers.stop }),
        deleteDesign: async (body) => {
          const res = await (writers.remove ?? postDeleteDesign)(body)
          if (!res.refused) navigate("/epics")
          return res
        },
      },
    },
  )

  const stopOther = (s: SessionInfo) => {
    const stop = writers.stop ?? postStopSession
    const label = s.name || s.id
    const target = anchor()
    openConfirm({
      actionId: "stop",
      entityName: label,
      group: "lifecycle",
      confirmLabel: "Stop",
      dismissLabel: "Cancel",
      title: `Stop ${label}`,
      body: { lines: [`Stop ${label}? Its agent ends.`] },
      surface: target ? { mode: "anchor", anchor: target } : { mode: "modal" },
      onConfirm: async () => {
        const res = await stop({ id: s.id, state: rowToken(s) })
        conn.refresh()
        return res
      },
    })
  }

  const groups: MenuEntry[][] = []
  const agent = shown && agentSection(shown, wide)
  if (agent) groups.push(agent)
  if (shown && live.length > 1)
    groups.push(liveSessionEntries(live, shown, false, onSelect, stopOther, true))
  groups.push(...registry)
  return (
    <span ref={wrap} style={{ display: "contents" }}>
      <GxMenu
        trigger={<MoreActionsButton />}
        items={menuGroups(groups)}
        onOpenChange={onOpenChange}
      />
    </span>
  )
}
