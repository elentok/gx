import { createOpenStore } from "./externalStore"

// Shared by the palette's Cmd/Ctrl+K handler and the header's search button. Kept outside React state
// so opening the palette re-renders only the palette, not AppShell.
const store = createOpenStore()

export const setJumpOpen = store.set
export const useJumpOpen = store.use
