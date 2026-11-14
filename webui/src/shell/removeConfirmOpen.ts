import type { SessionInfo } from "../generated/server"
import type { DeleteDraft } from "../design/drafts"
import { createExternalStore } from "./externalStore"

/** The draft a Remove names. */
export interface DraftRemoveTarget {
  id: string
  label: string
}

export type RemoveConfirmRequest =
  | {
      session: SessionInfo
      /** Each opener keeps its own navigation. */
      onRemoved: () => void
    }
  | {
      draft: DraftRemoveTarget
      onRemoved: () => void
      /** Overrides the real writer; tests inject a fake. */
      remove?: DeleteDraft
    }
  | null

// App-level state for the remove confirm, kept outside React like newItemOpen so the row menu, the
// agent page menu and the `x` chord open one dialog and only the host re-renders.
const store = createExternalStore<RemoveConfirmRequest>(null)

export const openRemoveConfirm = (open: NonNullable<RemoveConfirmRequest>) => store.set(open)
export const closeRemoveConfirm = () => store.set(null)
export const useRemoveConfirmRequest = store.use
