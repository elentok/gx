import { createOpenStore } from "./externalStore"

// Open state of the Command palette (Cmd/Ctrl+P), kept outside React state like jumpOpen.ts.
const store = createOpenStore()

export const setCommandOpen = store.set
export const useCommandOpen = store.use
