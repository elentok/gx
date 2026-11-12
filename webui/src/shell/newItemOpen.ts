import { createExternalStore } from "./externalStore"

export type NewItemRequest = { kind: "design" } | { kind: "agent" } | null

// App-level state for the New design / New chat forms, kept outside React like jumpOpen so any
// component (footer, pages, the palette) can open a form and only the host re-renders.
const store = createExternalStore<NewItemRequest>(null)

export const openNewDesign = () => store.set({ kind: "design" })
export const openNewAgent = () => store.set({ kind: "agent" })
export const closeNewItem = () => store.set(null)
export const useNewItemRequest = store.use
