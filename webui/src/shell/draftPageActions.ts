/** What the Chord box can ask of the open draft page. */
export interface DraftPageActions {
  /** Removes at once when the draft is empty, else shows the footer's Keep and Remove pair. */
  remove: () => void
  rename: () => void
  /** True when the draft has no prompt, images or slug. */
  isEmpty: () => boolean
  focusPrompt: () => void
  /** Opens the History popup. */
  openHistory: () => void
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

/** What New chat draft asks of an open chat draft page. Apart from the design page's so New design never lands here. */
export interface ChatDraftPageActions {
  /** True when the brief is blank. */
  isEmpty: () => boolean
  focusBrief: () => void
  /** Opens the History popup. */
  openHistory: () => void
}

let currentChat: ChatDraftPageActions | null = null

export function registerChatDraftPage(actions: ChatDraftPageActions): () => void {
  currentChat = actions
  return () => {
    if (currentChat === actions) currentChat = null
  }
}

export const chatDraftPageActions = () => currentChat
