import { postTargetsTo, rowToken } from "../design/open"
import { TargetDone, type SessionInfo, type TargetResult } from "../generated/server"
import { showToast } from "../ui/toaster"
import { WriteError } from "../useWrite"

/** The workspace writers: one POST for all `targets`, one result per target in the server's answer. */
const postTargets = (
  route: string,
  body:
    | { name: string; targets: string[] }
    | { targets: string[]; dry_run?: boolean; yes?: boolean },
) => postTargetsTo(`/v1/workspaces/${route}`, body)

export const bulkMove = (targets: string[], to: string) =>
  postTargets("move", { name: to, targets })

/** `dryRun` asks what a hold would stop without holding; `yes` confirms the stop of live sessions. */
export const bulkHold = (targets: string[], opts: { dryRun?: boolean; yes?: boolean } = {}) =>
  postTargets("hold", {
    targets,
    ...(opts.dryRun ? { dry_run: true } : {}),
    ...(opts.yes ? { yes: true } : {}),
  })

export const bulkUnhold = (targets: string[]) => postTargets("unhold", { targets })

/** Each session goes with its row token, so a row that changed since it was selected is refused as stale. */
const postSessions = (path: string, sessions: SessionInfo[], yes: boolean) =>
  postTargetsTo(path, {
    targets: sessions.map((s) => ({ id: s.id, state: rowToken(s) })),
    yes,
  })

/** `dryRun` asks what an archive would stop and whether each epic is on trunk, without archiving; `yes` confirms the stop of live sessions. */
export const bulkArchive = (targets: string[], opts: { dryRun?: boolean; yes?: boolean } = {}) =>
  postTargetsTo("/v1/epics/archive", {
    targets,
    ...(opts.dryRun ? { dry_run: true } : {}),
    ...(opts.yes ? { yes: true } : {}),
  })

/** Targets are archive addresses; a clash with a live epic comes back renamed in `new_address`. */
export const bulkUnarchive = (targets: string[]) =>
  postTargetsTo("/v1/epics/unarchive", { targets })

/** Unarchives and reports refusals and renames as toasts. Returns the results, or undefined when the call itself failed. */
export async function unarchiveWithToasts(
  targets: string[],
  nameOf: (target: string) => string,
): Promise<TargetResult[] | undefined> {
  try {
    const results = await bulkUnarchive(targets)
    for (const r of results) {
      if (r.status !== TargetDone)
        showToast({
          type: "error",
          title: `${nameOf(r.target)}: ${r.message || r.reason || "refused"}`,
        })
      else if (r.new_address)
        showToast({ type: "info", title: `Unarchived as ${r.new_address.split(":").pop()}` })
    }
    return results
  } catch (err) {
    showToast({
      type: "error",
      title: err instanceof WriteError ? err.message : "Could not reach gx. Try again.",
    })
    return undefined
  }
}

export const bulkStop = (sessions: SessionInfo[]) =>
  postSessions("/v1/sessions/stop", sessions, false)

/** Remove confirms with yes: the server stops a live session first. */
export const bulkRemove = (sessions: SessionInfo[]) =>
  postSessions("/v1/sessions/remove", sessions, true)
