export interface AnswerBody {
  address: string;
  state: string;
  /** An option id, or "yes"/"no". Empty for a free-text answer. */
  choice?: string;
  text?: string;
  note?: string;
}

export type AnswerResult =
  | { outcome: string; queue?: { address: string }[] }
  | { refused: true; reason: string; message: string };

export const OutcomeDelivered = "delivered";
export const OutcomeQueued = "queued";
export const OutcomeAdopted = "adopted";
export const ReasonStale = "stale";

/** What the page shows after a successful send; seq says which question it answered. */
export interface Sent {
  seq: number;
  outcome: string;
  /** 1-based place in the queue, for a queued answer. */
  position?: number;
}

export function sentOf(seq: number, address: string, res: { outcome: string; queue?: { address: string }[] }): Sent {
  const place = (res.queue ?? []).findIndex((q) => q.address === address) + 1;
  return { seq, outcome: res.outcome, ...(place > 0 ? { position: place } : {}) };
}

export type PostAnswer = (body: AnswerBody) => Promise<AnswerResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postAnswer: PostAnswer = async (body) => {
  const res = await fetch("/v1/tickets/answer", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (result && ("outcome" in result || result.refused)) return result;
  throw new Error(`answer: HTTP ${res.status}`);
};
