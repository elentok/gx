import type { Snapshot } from "./generated/server";

// The wire types come from the Go server (make web-types).
export type {
  EpicState,
  EpicSummary,
  FinishedEpic,
  QueueMode,
  Snapshot,
  SummaryTicket,
  TicketInfo,
} from "./generated/server";

export async function fetchSnapshot(): Promise<Snapshot> {
  const res = await fetch("/v1/snapshot");
  if (!res.ok) throw new Error(`snapshot: HTTP ${res.status}`);
  return res.json();
}
