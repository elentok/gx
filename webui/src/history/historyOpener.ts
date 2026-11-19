// The composer registers while mounted; the key handler lives outside it, so they meet here.
let current: (() => void) | null = null

export function registerHistoryOpener(open: () => void): () => void {
  current = open
  return () => {
    if (current === open) current = null
  }
}

/** Opens the mounted composer's History popup; false when no composer offers one. */
export function openHistory(): boolean {
  if (!current) return false
  current()
  return true
}
