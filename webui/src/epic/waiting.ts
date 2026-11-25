import type { EpicBlocker } from "../generated/server"

// The line a Waiting epic shows: the first blocker's bare name, then a count of the rest.
// The server sorts the list (not-moving first, then by name), so the first entry is the one to name.
export function waitingWords(blockers: readonly EpicBlocker[]): string {
  const first = blockers[0]?.name ?? ""
  const more = blockers.length - 1
  return more > 0 ? `Waiting on ${first} and ${more} more` : `Waiting on ${first}`
}
