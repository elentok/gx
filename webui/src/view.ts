import type { EpicState, QueueMode } from "./snapshot";

// The labels match epicStateLabels in ui/tickets/queue_header.go except finished: the web says "Merged", the TUI
// still says "Finished" (gx:follow-ups/46).
const STATE_LABELS: Record<EpicState, string> = {
  paused: "Paused",
  running: "Running",
  merging: "Merging",
  waiting: "Waiting for a slot",
  "awaiting-merge": "Awaiting merge",
  finished: "Merged",
};

// Only Waiting changes with the queue mode: a paused queue is why
// nothing starts. A running epic keeps its label.
export function epicBadge(state: EpicState, mode?: QueueMode): string {
  if (state === "waiting" && mode === "paused") {
    return `Waiting: queue ${mode}`;
  }
  return STATE_LABELS[state];
}
