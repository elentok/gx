import { useSyncExternalStore } from "react"

export type NewItemRequest = { kind: "design" } | { kind: "agent" } | null

// App-level state for the New design / New chat forms, kept outside React like jumpOpen so any
// component (footer, pages, the palette) can open a form and only the host re-renders.
let request: NewItemRequest = null
const listeners = new Set<() => void>()

function set(next: NewItemRequest) {
  request = next
  listeners.forEach((l) => l())
}

export const openNewDesign = () => set({ kind: "design" })
export const openNewAgent = () => set({ kind: "agent" })
export const closeNewItem = () => set(null)

export function useNewItemRequest(): NewItemRequest {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => request,
  )
}
