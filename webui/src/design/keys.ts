import { apiFetch } from "../apiFetch";
import type { SessionKeysRequest, SessionResult } from "../generated/server";
import { readWrite } from "./open";

export type PostKeys = (body: SessionKeysRequest) => Promise<SessionResult>;

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
export const postKeys: PostKeys = async (body) => {
  const res = await apiFetch("/v1/sessions/keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return readWrite(res);
};
