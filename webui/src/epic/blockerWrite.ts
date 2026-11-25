import { apiFetch } from "../apiFetch"
import type { EpicBlockerRequest, EpicBlockerResult } from "../generated/server"
import { WriteError } from "../useWrite"

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
async function postBlocker(body: EpicBlockerRequest): Promise<EpicBlockerResult> {
  const res = await apiFetch("/v1/epics/blocker", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
  const result = await res.json().catch(() => null)
  if (result) return result
  throw new WriteError(`HTTP ${res.status}`)
}

/** Adds one blocker; the server keeps every other stored entry. */
export const postBlockerAdd = (address: string, blocker: string) =>
  postBlocker({ address, blocker })

/** Removes one blocker, a stored entry the page shows as an error chip included. */
export const postBlockerRemove = (address: string, blocker: string) =>
  postBlocker({ address, blocker, remove: true })
