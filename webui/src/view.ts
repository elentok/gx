import type { EpicState, QueueMode } from "./snapshot";

// The labels must match epicStateLabels in ui/tickets/queue_header.go.
const STATE_LABELS: Record<EpicState, string> = {
  paused: "Paused",
  running: "Running",
  merging: "Merging",
  waiting: "Waiting for a slot",
  "awaiting-merge": "Awaiting merge",
  finished: "Finished",
};

// Only Waiting changes with the queue mode: a paused queue is why
// nothing starts. A running epic keeps its label.
export function epicBadge(state: EpicState, mode?: QueueMode): string {
  if (state === "waiting" && mode === "paused") {
    return `Waiting: queue ${mode}`;
  }
  return STATE_LABELS[state];
}
