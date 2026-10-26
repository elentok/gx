import {
  CardKindDesign,
  CardKindFollowUps,
  CardKindImplementation,
  CardKindOneOff,
  EpicListFinished,
  EpicListMerging,
  EpicListRunning,
  SectionNeedsYou,
  type EpicListRow,
} from "../generated/server";
import type { Kind, Tone } from "../ui/tone";

export const RING_KIND: Record<EpicListRow["card_kind"] & string, Kind> = {
  [CardKindDesign]: "design",
  [CardKindImplementation]: "implementation",
  [CardKindOneOff]: "oneoff",
  [CardKindFollowUps]: "followups",
};

const PILL_TONE: Record<string, Tone> = {
  Question: "waiting",
  Permission: "waiting",
  Paused: "paused",
  Deadlocked: "deadlocked",
  "Merge blocked": "deadlocked",
  "Awaiting merge": "done",
};

export const needsYouTone = (row: EpicListRow): Tone => PILL_TONE[row.state_pill ?? ""] ?? "waiting";

export function ringTone(row: EpicListRow): Tone {
  if (row.section === SectionNeedsYou) return needsYouTone(row);
  if (row.state === EpicListFinished) return "done";
  return isLive(row) ? "running" : "muted";
}

export const isCreatingTickets = (row: EpicListRow) => row.status_words?.startsWith("Creating tickets") ?? false;

export const isLive = (row: EpicListRow) =>
  row.state === EpicListRunning || row.state === EpicListMerging || isCreatingTickets(row);

export const ringProgress = (row: EpicListRow) =>
  (row.ring_progress ?? (row.total ? (row.done / row.total) * 100 : 0)) / 100;
