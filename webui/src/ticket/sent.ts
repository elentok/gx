import { useSyncExternalStore } from "react"
import type { Sent } from "./answer"

// The send outcome per ticket, in memory only: it outlives the page that sent it (the answer page
// goes back to the ticket page) but a reload clears it.
const sents = new Map<string, Sent>()
const listeners = new Set<() => void>()

export function recordSent(address: string, sent: Sent) {
  sents.set(address, sent)
  for (const l of listeners) l()
}

const subscribe = (l: () => void) => {
  listeners.add(l)
  return () => void listeners.delete(l)
}

export const useSent = (address: string): Sent | null =>
  useSyncExternalStore(
    subscribe,
    () => sents.get(address) ?? null,
    () => null,
  )

/** For tests: forget every recorded send. */
export const clearSents = () => {
  sents.clear()
  for (const l of listeners) l()
}
