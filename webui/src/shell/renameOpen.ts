import { useSyncExternalStore } from "react"
import { isChat } from "../agents/sessionWrites"
import type { SessionInfo } from "../generated/server"
import type { MenuEntry } from "../ui/menu/Menu"
import type { RenameSession } from "../design/open"

export type RenameRequest = {
  session: SessionInfo
  /** Overrides the real writer; tests inject a fake. */
  rename?: RenameSession
} | null

// App-level state for the rename popup, kept outside React like removeConfirmOpen so the menu, the
// chord and the palette open one popup and only the host re-renders.
let request: RenameRequest = null
const listeners = new Set<() => void>()

function set(next: RenameRequest) {
  request = next
  listeners.forEach((l) => l())
}

export const openRename = (open: NonNullable<RenameRequest>) => set(open)
export const closeRename = () => set(null)

/** The Rename menu item both chat menus share: empty unless the session is a chat. */
export function renameEntry(session: SessionInfo | undefined): MenuEntry[] {
  if (!session || !isChat(session)) return []
  return [{ value: "rename", label: "Rename", onSelect: () => openRename({ session }) }]
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
