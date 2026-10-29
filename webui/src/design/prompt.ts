import { apiFetch } from "../apiFetch";
import type { SessionPromptRequest, SessionResult } from "../generated/server";
import { readWrite } from "./open";

export type PostPrompt = (body: SessionPromptRequest) => Promise<SessionResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postPrompt: PostPrompt = async (body) => {
  const res = await apiFetch("/v1/sessions/prompt", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return readWrite(res);
};
