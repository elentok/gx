import { apiFetch } from "../apiFetch";
import type { ChecksStatus, ProjectChecks } from "../generated/server";

export type { ChecksStatus, ProjectChecks };

export type FetchChecksStatus = () => Promise<ChecksStatus>;

export const fetchChecksStatus: FetchChecksStatus = async () => {
  const res = await apiFetch("/v1/checks/status");
  if (!res.ok) throw new Error(`checks: HTTP ${res.status}`);
  return res.json();
};

/** How many kinds gx has a command to suggest for. */
export const suggestionCount = (p: ProjectChecks): number => p.kinds.filter((k) => k.recommended).length;
