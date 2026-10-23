export interface EpicWriteBody {
  address: string;
  /** The web mux refuses a queue write without a state token; the epic's list state is enough. */
  state: string;
}

export type EpicWriteResult = { refused?: false } | { refused: true; reason: string; message: string };

export type PostEpicWrite = (body: EpicWriteBody) => Promise<EpicWriteResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
async function postEpicWrite(path: string, body: EpicWriteBody): Promise<EpicWriteResult> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (res.ok && result) return result;
  throw new Error(`HTTP ${res.status}`);
}

export const postQueueAdd: PostEpicWrite = (body) => postEpicWrite("/v1/queue/add", body);

/** The merge route takes the same address and state token as a queue add. */
export const postEpicMerge: PostEpicWrite = (body) => postEpicWrite("/v1/epics/merge", body);
