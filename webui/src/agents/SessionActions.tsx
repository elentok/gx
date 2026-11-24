import { Box } from "@chakra-ui/react"
import type { PostPrompt } from "../design/prompt"
import { usePromptSend } from "../design/usePromptSend"
import { postStopSession, type StopSession } from "../design/open"
import { agentSection } from "../design/SessionControls"
import type { SessionInfo } from "../generated/server"
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { useWide } from "../useWide"
import { renameEntry } from "../shell/renameOpen"
import { chatEntity } from "../actions/entity"
import { useSnapshotField } from "../connection"
import { useEntityActions } from "../workspaces/itemActions"

/** The "⋯" menu: the Agent block, plus Stop (while live) and Remove for a session a person manages. */
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
  // The Agent block, Promote and Rename are built here; the registry supplies the rest.
  const registry = useEntityActions(
    { ...chatEntity(session), surface: "page", workspace },
    workspaces,
    { hide: ["switch-model", "promote", "rename"] },
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
  if (promote) {
    const off =
      promote.reason ??
      (promote.hasMessages
        ? undefined
        : "No messages yet. Promote turns on after the first message.")
    groups.push([
      {
        value: "promote",
        label: "Promote to Design",
        hint: off,
        disabled: !promoteSend.enabled || off !== undefined,
        onSelect: () => promoteSend.send({ promote: true }),
      },
    ])
  }
  // Rename keeps its own entry: the registry offers it to Design and ticket sessions too.
  const rename = renameEntry(session)
  if (rename.length > 0) groups.push(rename)
  groups.push(...registry.groups)
  // A queue session with no tier, model or effort has nothing to put in the menu.
  if (groups.length === 0) return null
  return (
    <Box>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} />
    </Box>
  )
}
