/** What the Chord box can ask of the open draft page. */
export interface DraftPageActions {
  /** Removes at once when the draft is empty, else shows the footer's Keep and Remove pair. */
  remove: () => void
  rename: () => void
  /** True when the draft has no prompt, images or slug. */
  isEmpty: () => boolean
  focusPrompt: () => void
}

// The page registers while mounted; the Chord host lives outside it, so they meet here.
let current: DraftPageActions | null = null

export function registerDraftPage(actions: DraftPageActions): () => void {
  current = actions
  return () => {
    if (current === actions) current = null
  }
}

export const draftPageActions = () => current
