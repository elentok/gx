export interface AnswerBody {
  address: string;
  state: string;
  /** An option id, or "yes"/"no". Empty for a free-text answer. */
  choice?: string;
  text?: string;
  note?: string;
}

export type AnswerResult = { outcome: string } | { refused: true; reason: string; message: string };

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
