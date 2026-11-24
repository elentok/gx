import { useMemo } from "react"
import { useNavigate } from "react-router"
import type { Entity } from "../actions/entity"
import { useOpenEntity } from "../actions/openEntity"
import { useActions, type ReadyAction } from "../actions/useActions"
import { useConnectionContext } from "../connection"
import { REMOVED_NOTE } from "../design/useDraftLifecycle"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import { chatDraftPageActions, draftPageActions } from "./draftPageActions"
import { openWorkspacePicker } from "./workspacePickerOpen"

/**
 * What a chord or palette row does for an action. Move has no submenu on a chord, so it opens the
 * picker. A draft page's Rename focuses its title, which only the page can do.
 */
function runOf(a: ReadyAction, entity: Entity | undefined): () => void {
  if (a.id === "move")
    return () => openWorkspacePicker({ name: entity?.name ?? "", items: a.items ?? [] })
  if (a.id === "rename") {
    const page =
      entity?.kind === "design-draft"
        ? draftPageActions
        : entity?.kind === "chat-draft"
          ? chatDraftPageActions
          : undefined
    if (page) return () => page()?.rename()
  }
  return a.run
}

/** The open draft page's own state, which the Remove confirm needs and the snapshot lacks. */
const draftPageOf = (entity: Entity | undefined) =>
  entity?.kind === "design-draft"
    ? draftPageActions()
    : entity?.kind === "chat-draft"
      ? chatDraftPageActions()
      : null

/**
 * The open page's entity and its registry actions, each ready to run as a modal (a chord or a
 * palette row has no button to anchor on). A removed chat leaves its page.
 */
export function useOpenEntityActions() {
  const navigate = useNavigate()
  const open = useOpenEntity()
  const { workspaces } = useWorkspaces(useConnectionContext())
  const isDraft = open.entity?.kind === "design-draft" || open.entity?.kind === "chat-draft"
  // The page can hold text the server has not saved yet, so emptiness is read when Remove runs.
  const entity = useMemo(
    () =>
      open.entity && isDraft
        ? {
            ...open.entity,
            get draftEmpty() {
              return draftPageOf(open.entity)?.isEmpty()
            },
          }
        : open.entity,
    [open.entity, isDraft],
  )
  const deleteNow = async () => {
    draftPageOf(entity)?.deleteNow()
    return { kind: "deleted" as const }
  }
  const { actions } = useActions(entity, {
    surface: { mode: "modal" },
    workspaces,
    writers: { deleteDraft: deleteNow, deleteChatDraft: deleteNow },
    onRemoved: () =>
      isDraft
        ? navigate("/epics", entity?.kind === "chat-draft" ? { state: { note: REMOVED_NOTE } } : {})
        : navigate("/agents"),
  })
  const ready = useMemo(
    () => actions.map((a) => ({ ...a, run: runOf(a, entity) })),
    [actions, entity],
  )
  return { ...open, entity, actions: ready }
}
