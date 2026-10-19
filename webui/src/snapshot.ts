// Mirrors the fields of server.Snapshot the shell reads; grow it as views need more.
export interface TicketInfo {
  address: string;
  title: string;
  status: string;
  rendered_status: string;
  type: string;
  blocked_by?: string[];
}

// server.EpicSummary.State values.
export type EpicState = "awaiting-merge" | "running" | "waiting" | "parked" | "deadlocked";

// server.SummaryTicket: an active ticket carries claimed_at, a parked one recovery and reason.
export interface SummaryTicket {
  address: string;
  title: string;
  status: string;
  claimed_at?: string;
  recovery?: string;
  reason?: string;
}

// server.EpicSummary
export interface EpicSummary {
  address: string;
  name: string;
  project: string;
  state: EpicState;
  done: number;
  total: number;
  active: SummaryTicket[];
  parked: SummaryTicket[];
  reason?: string;
}

// server.FinishedEpic
export interface FinishedEpic {
  address: string;
  name: string;
  project: string;
  done: number;
  total: number;
  completed_at: string;
}

export interface Snapshot {
  seq: number;
  mode?: string;
  tickets: TicketInfo[];
  epics: EpicSummary[];
  recently_finished: FinishedEpic[];
}

export async function fetchSnapshot(): Promise<Snapshot> {
  const res = await fetch("/v1/snapshot");
  if (!res.ok) throw new Error(`snapshot: HTTP ${res.status}`);
  return res.json();
}
