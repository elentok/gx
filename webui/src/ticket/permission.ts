export type Decision = "allow" | "deny";

export interface DecisionBody {
  address: string;
  decision: Decision;
  /** The prompt the person saw; the server refuses the decision if another is open now. */
  request_id: string;
  /** Sent on Deny only. */
  text?: string;
}

export type DecisionResult = { refused?: false } | { refused: true; reason: string; message: string };

export type PostDecision = (body: DecisionBody) => Promise<DecisionResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postDecision: PostDecision = async (body) => {
  const res = await fetch("/v1/agents/answer", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (res.ok || (result && result.refused)) return result ?? {};
  throw new Error(`decision: HTTP ${res.status}`);
};
