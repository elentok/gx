import { createExternalStore } from "./externalStore"

export type NewItemRequest = { kind: "agent" } | null

// App-level state for the New chat form, kept outside React like jumpOpen so any component
// (footer, pages, the palette) can open it and only the host re-renders. New design has no form:
// see `startNewDesign`.
const store = createExternalStore<NewItemRequest>(null)

export const openNewAgent = () => store.set({ kind: "agent" })
export const closeNewItem = () => store.set(null)
export const useNewItemRequest = store.use
