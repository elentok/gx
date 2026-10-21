export interface QueueAddBody {
  address: string;
  /** The web mux refuses a queue write without a state token; the epic's list state is enough. */
  state: string;
}

export type QueueAddResult = { refused?: false } | { refused: true; reason: string; message: string };

export type PostQueueAdd = (body: QueueAddBody) => Promise<QueueAddResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postQueueAdd: PostQueueAdd = async (body) => {
  const res = await fetch("/v1/queue/add", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (res.ok && result) return result;
  throw new Error(`queue add: HTTP ${res.status}`);
};
