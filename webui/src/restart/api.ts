import { apiFetch } from "../apiFetch";
import type { RestartCheckReport, RestartRequest, RestartResult } from "../generated/server";

export type FetchRestartCheck = () => Promise<RestartCheckReport>;
export type PostRestart = (body: RestartRequest) => Promise<RestartResult>;

export const fetchRestartCheck: FetchRestartCheck = async () => {
  const res = await apiFetch("/v1/restart/check");
  if (!res.ok) throw new Error((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  return res.json();
};

export const postRestart: PostRestart = async (body) => {
  const res = await apiFetch("/v1/restart", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
  return res.json();
};
