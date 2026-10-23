import type { SessionPromptRequest, SessionResult } from "../generated/server";

export type PostPrompt = (body: SessionPromptRequest) => Promise<SessionResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postPrompt: PostPrompt = async (body) => {
  const res = await fetch("/v1/sessions/prompt", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await res.json().catch(() => null);
  if (result && (res.ok || result.refused)) return result;
  throw new Error(`prompt: HTTP ${res.status}`);
};
