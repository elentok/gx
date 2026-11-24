import { Box } from "@chakra-ui/react"
import { useRef } from "react"
import { renameEntry } from "../shell/renameOpen"
import type { PostPrompt } from "../design/prompt"
import { usePromptSend } from "../design/usePromptSend"
import { postStopSession, type StopSession } from "../design/open"
import { agentSection } from "../design/SessionControls"
import type { SessionInfo } from "../generated/server"
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { useWide } from "../useWide"
import { chatEntity } from "../actions/entity"
import { useSnapshotField } from "../connection"
import { useEntityActions } from "../workspaces/itemActions"

/** The "⋯" menu: the Agent block, then the registry's actions for the chat, confirming on the ⋯. */
export function SessionActions({
  session,
  onRemoved,
  stop = postStopSession,
  promote,
}: {
  session: SessionInfo
  onRemoved: () => void
  stop?: StopSession
  /** Adds "Promote to Design"; absent for a gx-driven agent. */
  promote?: {
    lastMessage?: string
    hasMessages: boolean
    connected: boolean
    /** Why the composer cannot send; the caller decides the text. */
    reason?: string
    post?: PostPrompt
    onStale?: () => void
  }
}) {
  const workspaces = useSnapshotField((s) => s.workspaces) ?? []
  const workspace = useSnapshotField(
    (s) => s.placements?.find((p) => p.session === session.id)?.workspace,
  )
  const wrap = useRef<HTMLSpanElement>(null)
  // The registry supplies every item, in its order and on its anchor; only the Agent block and
  // Promote's disabled reasons are built here.
  const registry = useEntityActions(
    { ...chatEntity(session), surface: "page", workspace },
    workspaces,
    {
      // Rename is offered only where the menu already offers it (not to Design or ticket sessions).
      hide: [...(promote ? [] : ["promote"]), ...(renameEntry(session).length ? [] : ["rename"])],
      anchor: () => wrap.current?.querySelector("button"),
    },
    { onRemoved, writers: { stop } },
  )
  const promoteSend = usePromptSend({
    session,
    lastMessage: promote?.lastMessage,
    connected: promote?.connected ?? false,
    post: promote?.post,
    onStale: promote?.onStale,
    toastRefusal: true,
  })
  const wide = useWide()
  const groups: MenuEntry[][] = []
  const agent = agentSection(session, wide)
  if (agent) groups.push(agent)
  const off = promote
    ? (promote.reason ??
      (promote.hasMessages
        ? undefined
        : "No messages yet. Promote turns on after the first message."))
    : undefined
  // The chat's own send path replaces the registry's Promote entry in place.
  const promoteEntry: MenuEntry = {
    value: "promote",
    label: "Promote to Design",
    hint: off,
    disabled: !promoteSend.enabled || off !== undefined,
    onSelect: () => promoteSend.send({ promote: true }),
  }
  groups.push(
    ...registry.groups.map((g) =>
      g.map((e) => ("value" in e && e.value === "promote" ? promoteEntry : e)),
    ),
  )
  // A queue session with no tier, model or effort has nothing to put in the menu.
  if (groups.length === 0) return null
  return (
    <Box>
      <span ref={wrap} style={{ display: "contents" }}>
        <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} />
      </span>
    </Box>
  )
}
