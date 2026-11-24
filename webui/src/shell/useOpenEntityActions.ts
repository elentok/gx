import { useMemo } from "react"
import { useNavigate } from "react-router"
import type { Entity } from "../actions/entity"
import { useActions, type ReadyAction } from "../actions/useActions"
import { useConnectionContext } from "../connection"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import { chatDraftPageActions, draftPageActions } from "./draftPageActions"
import { useOpenEntity } from "./chords"
import { openWorkspacePicker } from "./workspacePickerOpen"

/**
 * What a chord or palette row does for an action. A draft page owns its Rename (focus the title)
 * and its Remove (the footer pair or an instant delete), and Move has no submenu there, so it
 * opens the picker.
 */
function runOf(a: ReadyAction, entity: Entity | undefined): () => void {
  if (a.id === "move")
    return () => openWorkspacePicker({ name: entity?.name ?? "", items: a.items ?? [] })
  if (a.id === "rename" || a.id === "remove") {
    const page =
      entity?.kind === "design-draft"
        ? draftPageActions
        : entity?.kind === "chat-draft"
          ? chatDraftPageActions
          : undefined
    const id = a.id
    if (page) return () => page()?.[id]()
  }
  return a.run
}

/**
 * The open page's entity and its registry actions, each ready to run as a modal (a chord or a
 * palette row has no button to anchor on). A removed chat leaves its page.
 */
export function useOpenEntityActions() {
  const navigate = useNavigate()
  const open = useOpenEntity()
  const { entity } = open
  const { workspaces } = useWorkspaces(useConnectionContext())
  const { actions } = useActions(entity, {
    surface: { mode: "modal" },
    workspaces,
    onRemoved: () => navigate("/agents"),
  })
  const ready = useMemo(
    () => actions.map((a) => ({ ...a, run: runOf(a, entity) })),
    [actions, entity],
  )
  return { ...open, actions: ready }
}
