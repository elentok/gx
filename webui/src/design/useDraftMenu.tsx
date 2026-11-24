import { useRef, type ReactNode } from "react"
import type { Entity } from "../actions/entity"
import { draftEntity } from "../actions/openEntity"
import { useConnectionContext, useConnectionState } from "../connection"
import { GxMenu, menuGroups } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { useEntityActions } from "../workspaces/itemActions"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import type { ActionWriters } from "../actions/useActions"

interface Options {
  kind: "design-draft" | "chat-draft"
  id: string
  name: string
  /** The draft has nothing to lose, so Remove skips its confirm. */
  empty: boolean
  /** Stops the page's autosave and deletes the draft. */
  deleteNow: () => void
  /** Leaves the page once the draft is gone. */
  onRemoved: () => void
}

/**
 * The ⋯ menu of a draft page and its Remove action, both from the registry. The footer's Remove
 * button confirms on itself; a chord, which has no button, confirms as a modal.
 */
export function useDraftMenu({ kind, id, name, empty, deleteNow, onRemoved }: Options) {
  const conn = useConnectionContext()
  const { workspaces } = useWorkspaces(conn)
  const snapshot = useConnectionState(conn).snapshot
  const row =
    kind === "chat-draft"
      ? snapshot?.chat_drafts?.find((d) => d.id === id)
      : snapshot?.design_drafts?.find((d) => d.id === id)
  const wrap = useRef<HTMLSpanElement>(null)
  const entity: Entity = { ...draftEntity(row, kind, id), name, draftEmpty: empty }
  const key = entity.key
  const writers: Partial<ActionWriters> = {
    deleteDraft: async () => {
      deleteNow()
      return { kind: "deleted" }
    },
    deleteChatDraft: async () => {
      deleteNow()
      return { kind: "deleted" }
    },
  }
  const { groups, get } = useEntityActions(
    entity,
    workspaces,
    {
      hide: [],
      page: key,
      anchor: () => wrap.current?.querySelector("button"),
    },
    {
      draft: { id, slug: "", project: "" },
      writers,
      onRemoved,
    },
  )
  const remove = (anchor: HTMLElement | null = null) => get("remove")?.run(anchor)
  const menu: ReactNode = (
    <span ref={wrap} style={{ display: "contents" }}>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} />
    </span>
  )
  return { menu, remove }
}
