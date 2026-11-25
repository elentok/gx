import { useCallback, useMemo, useSyncExternalStore } from "react"
import type { Answer } from "../ui/question-sheet/round"

// The unsent answer per ticket, in memory only: it outlives the answer page (leaving and coming
// back) but a reload clears it. It remembers the detail.state token it belongs to.
type Draft = { answer: Answer; state: string | undefined }

const drafts = new Map<string, Draft>()
const listeners = new Set<() => void>()
const emit = () => {
  for (const l of listeners) l()
}

const subscribe = (l: () => void) => {
  listeners.add(l)
  return () => void listeners.delete(l)
}

export function clearDraft(address: string) {
  if (drafts.delete(address)) emit()
}

/** For tests: forget every draft. */
export const clearDrafts = () => {
  drafts.clear()
  emit()
}

/**
 * The draft for one ticket's current question. A draft stored for another question loses its pick
 * and text and keeps its note, whether the question changed while mounted or before the mount.
 */
export function useDraft(
  address: string,
  state: string | undefined,
): [Answer, (a: Answer) => void] {
  const stored = useSyncExternalStore(
    subscribe,
    () => drafts.get(address),
    () => undefined,
  )
  const answer = useMemo<Answer>(
    () => (stored && stored.state === state ? stored.answer : { note: stored?.answer.note }),
    [stored, state],
  )
  const set = useCallback(
    (a: Answer) => {
      drafts.set(address, { answer: a, state })
      emit()
    },
    [address, state],
  )
  return [answer, set]
}
