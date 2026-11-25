import {
  EpicListCIFailing,
  EpicListDraft,
  EpicListMergeBlocked,
  EpicListNoOpenTickets,
  EpicListPaused,
  EpicListPRTextFailed,
  EpicListReady,
  type EpicBlocker,
  type EpicListState,
} from "../generated/server"

// The line a Waiting epic shows: the first blocker's bare name, then a count of the rest.
// The server sorts the list (invalid stored entries first, then not-moving, then by name), so the
// first entry is the one to name. An invalid entry has no name, so it reads as its stored text.
export function waitingWords(blockers: readonly EpicBlocker[]): string {
  const head = blockers[0]
  const first = (head?.invalid ? head.ref : head?.name) ?? ""
  const more = blockers.length - 1
  return more > 0 ? `Waiting on ${first} and ${more} more` : `Waiting on ${first}`
}

const STAYS_PUT = "This epic stays put until it is merged."

// Epics page labels of the states that stop a blocker from moving.
const STUCK_LABELS: Record<string, string> = {
  [EpicListPaused]: "Paused",
  [EpicListMergeBlocked]: "Merge blocked",
  [EpicListCIFailing]: "CI failing",
  [EpicListPRTextFailed]: "PR text failed",
}

const NOT_QUEUED_STATES: readonly EpicListState[] = [
  EpicListReady,
  EpicListDraft,
  EpicListNoOpenTickets,
]

export interface HintLine {
  text: string
  /** A stuck blocker reads in the Paused text tone, a not-queued one in the dim text. */
  stuck: boolean
}

// The reason a Waiting epic does not move, or null while its blocker is moving.
// A Waiting blocker over a stalled epic names that deeper epic and its state.
export function blockerHint(blockers: readonly EpicBlocker[]): HintLine | null {
  const first = blockers.find((b) => !b.invalid)
  if (!first) return null
  const target = first.hint ?? first
  const name = target.name
  if (!name || !target.state) return null
  const label = target.deadlocked ? "Deadlocked" : STUCK_LABELS[target.state]
  if (label) return { text: `${name} is ${label}. ${STAYS_PUT}`, stuck: true }
  if (NOT_QUEUED_STATES.includes(target.state))
    return { text: `${name} is not queued. ${STAYS_PUT}`, stuck: false }
  return null
}
