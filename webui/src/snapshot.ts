// Mirrors the fields of server.Snapshot the shell reads; grow it as views need more.
export interface TicketInfo {
  address: string;
  title: string;
  status: string;
  rendered_status: string;
  type: string;
  blocked_by?: string[];
}

export interface Snapshot {
  seq: number;
  mode?: string;
  tickets: TicketInfo[];
}

export async function fetchSnapshot(): Promise<Snapshot> {
  const res = await fetch("/v1/snapshot");
  if (!res.ok) throw new Error(`snapshot: HTTP ${res.status}`);
  return res.json();
}
