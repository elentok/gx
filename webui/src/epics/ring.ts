import {
  CardKindDesign,
  CardKindFollowUps,
  CardKindImplementation,
  CardKindOneOff,
  EpicListFinished,
  SectionNeedsYou,
  WorkKindDesign,
  type EpicListRow,
} from "../generated/server"
import type { Kind, Tone } from "../ui/tone"

export const RING_KIND: Record<EpicListRow["card_kind"] & string, Kind> = {
  [CardKindDesign]: "design",
  [CardKindImplementation]: "implementation",
  [CardKindOneOff]: "oneoff",
  [CardKindFollowUps]: "followups",
}

const PILL_TONE: Record<string, Tone> = {
  Question: "waiting",
  "Needs you": "waiting",
  Permission: "waiting",
  Paused: "paused",
  Stopped: "paused",
  Deadlocked: "deadlocked",
  "Merge blocked": "deadlocked",
  "Awaiting merge": "done",
}

/** A PR with every check green waits for a person on GitHub; the server sends it in the implementing section. */
export const isReadyForReview = (row: EpicListRow) =>
  !!row.pr && row.status_words === "Ready for review"

export const READY_FOR_REVIEW_REASON = "Waiting for you to merge the PR on GitHub."

export const needsYouTone = (row: EpicListRow): Tone => PILL_TONE[row.state_pill ?? ""] ?? "waiting"

/** A Follow-ups step pill (Verify, Route, ...) is running; only the waiting pills are amber. */
export const followUpsTone = (row: EpicListRow): Tone =>
  PILL_TONE[row.state_pill ?? ""] ?? "running"

export function ringTone(row: EpicListRow): Tone {
  if (row.card_kind === CardKindFollowUps && row.state_pill) {
    // The mock draws the same teal ring in every state; only the pill and border turn amber.
    const tone = followUpsTone(row)
    return tone === "waiting" ? "running" : tone
  }
  if (row.section === SectionNeedsYou) return needsYouTone(row)
  if (row.state === EpicListFinished || isReadyForReview(row)) return "done"
  return isLive(row) ? "running" : "muted"
}

/**
 * The mock draws a Follow-ups card's arc teal (the implementation colour) at Write and Done too.
 * A working Design is purple; the kind comes from the row's work_kind, never from card_kind,
 * because the joined card at Branch is "implementation" while the Design still works.
 */
export const DESIGN_ARC = "kind.design.fg"

/** The arc of a Design ring that is live and running; the one place the purple rule lives. */
export const designArc = (live: boolean, tone: Tone): string | undefined =>
  live && tone === "running" ? DESIGN_ARC : undefined

export const ringArc = (row: EpicListRow): string | undefined => {
  if (row.card_kind === CardKindFollowUps && ringTone(row) !== "muted")
    return "kind.implementation.fg"
  if (row.work_kind === WorkKindDesign) return designArc(isLive(row), ringTone(row))
  return undefined
}

/** A Design at Create tickets has no fraction yet; the server sends its bare working word. */
export const hasNoFractionYet = (row: EpicListRow) =>
  row.card_kind === CardKindDesign && !!row.working && row.working_word === "Creating tickets"

export const isLive = (row: EpicListRow) => !!row.working

export const ringProgress = (row: EpicListRow) =>
  (row.ring_progress ?? (row.total ? (row.done / row.total) * 100 : 0)) / 100
