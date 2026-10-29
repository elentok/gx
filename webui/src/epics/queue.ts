import { apiFetch } from "../apiFetch";
import { WriteError } from "../useWrite";

export interface EpicWriteBody {
  address: string;
  /** The web mux refuses a queue write without a state token; the epic's list state is enough. */
  state: string;
}

/** An epic merge's outcome: the root paused (`parked`, with `message`), or a merge ticket was forked. */
export type EpicWriteResult =
  | { refused?: false; merged?: boolean; parked?: boolean; park_kind?: string; message?: string; merge_ticket?: string }
  | { refused: true; reason: string; message: string };

export type PostEpicWrite = (body: EpicWriteBody) => Promise<EpicWriteResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
async function postEpicWrite(path: string, body: EpicWriteBody): Promise<EpicWriteResult> {
  const res = await apiFetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  const result = await res.json().catch(() => null);
  if (result) return result;
  throw new WriteError(`HTTP ${res.status}`);
}

export const postQueueAdd: PostEpicWrite = (body) => postEpicWrite("/v1/queue/add", body);

/** The merge route takes the same address and state token as a queue add. */
export const postEpicMerge: PostEpicWrite = (body) => postEpicWrite("/v1/epics/merge", body);

export const postEpicStashMerge: PostEpicWrite = (body) => postEpicWrite("/v1/epics/stash-merge", body);
