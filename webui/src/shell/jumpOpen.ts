import { createExternalStore } from "./externalStore"

// Shared by the palette's Cmd/Ctrl+K handler and the header's search button. Kept outside React state
// so opening the palette re-renders only the palette, not AppShell.
const store = createExternalStore(false)

export function setJumpOpen(next: boolean | ((o: boolean) => boolean)) {
  store.set(typeof next === "function" ? next(store.get()) : next)
}

export const useJumpOpen = store.use
