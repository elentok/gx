import type { TargetResult, TargetsResult } from "../generated/server";
import { postWorkspace } from "../workspaces/postWorkspace";

/** The many-target writers: one POST for all `targets`, one result per target in the server's answer. */
async function postTargets(route: string, body: { name: string; targets: string[] } | { targets: string[]; dry_run?: boolean; yes?: boolean }): Promise<TargetResult[]> {
  const { results } = await postWorkspace<TargetsResult>(route, body);
  return results ?? [];
}

export const bulkMove = (targets: string[], to: string) => postTargets("move", { name: to, targets });

/** `dryRun` asks what a hold would stop without holding; `yes` confirms the stop of live sessions. */
export const bulkHold = (targets: string[], opts: { dryRun?: boolean; yes?: boolean } = {}) =>
  postTargets("hold", { targets, ...(opts.dryRun ? { dry_run: true } : {}), ...(opts.yes ? { yes: true } : {}) });

export const bulkUnhold = (targets: string[]) => postTargets("unhold", { targets });
