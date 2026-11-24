import { useSyncExternalStore } from "react"
import { isChat } from "../agents/sessionWrites"
import type { DesignDraftRow, SessionInfo } from "../generated/server"
import type { SaveDraftFields } from "../design/drafts"
import type { MenuEntry } from "../ui/menu/Menu"
import type { RenameSession } from "../design/open"
import type { ChatDraftsClient } from "../design/chatDrafts"

/** What a slug write needs: it sends the project along so the write keeps it. */
export type DraftIdentity = Pick<DesignDraftRow, "id" | "slug" | "project">

export type RenameRequest =
  | {
      session: SessionInfo
      /** Overrides the real writer; tests inject a fake. */
      rename?: RenameSession
    }
  | {
      draft: DraftIdentity
      /** Overrides the real slug write; tests inject a fake. */
      saveFields?: SaveDraftFields
    }
  | {
      chatDraft: ChatDraftIdentity
      /** Overrides the real chat draft calls; tests inject a fake. */
      drafts?: ChatDraftsClient
    }
  | null

/** A chat draft row: its id and the name the row shows. */
export interface ChatDraftIdentity {
  id: string
  name: string
}

// App-level state for the rename popup, kept outside React like the confirm request so the menu, the
// chord and the palette open one popup and only the host re-renders.
let request: RenameRequest = null
const listeners = new Set<() => void>()

function set(next: RenameRequest) {
  request = next
  listeners.forEach((l) => {
    l()
  })
}

export const openRename = (open: NonNullable<RenameRequest>) => set(open)
export const closeRename = () => set(null)

/** The Rename menu item both chat menus share: empty unless the session is a chat. */
export function renameEntry(session: SessionInfo | undefined): MenuEntry[] {
  if (!session || !isChat(session)) return []
  return [{ value: "rename", label: "Rename", onSelect: () => openRename({ session }) }]
}

/** The draft row's Rename item. */
export function draftRenameEntry(draft: DraftIdentity): MenuEntry[] {
  return [{ value: "rename", label: "Rename", onSelect: () => openRename({ draft }) }]
}

export function useRenameRequest(): RenameRequest {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => request,
  )
}
