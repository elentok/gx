import { useSyncExternalStore } from "react"
import type { ReadyItem } from "../actions/useActions"

export type WorkspacePickerRequest = {
  /** What is being moved, for the dialog's heading. */
  name: string
  /** Every workspace, the current one marked; `run` posts the move. */
  items: ReadyItem[]
}

// App-level state like modelPickerOpen: the Move chord opens one picker and only its host re-renders.
let request: WorkspacePickerRequest | null = null
const listeners = new Set<() => void>()

function set(next: WorkspacePickerRequest | null) {
  request = next
  listeners.forEach((l) => {
    l()
  })
}

export const openWorkspacePicker = (open: WorkspacePickerRequest) => set(open)
export const closeWorkspacePicker = () => set(null)

export function useWorkspacePickerRequest(): WorkspacePickerRequest | null {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => request,
  )
}
