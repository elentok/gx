import { apiFetch } from "../apiFetch"
import type { HoldRequest, WorkspaceRequest } from "../generated/server"
import { WriteError } from "../useWrite"

/** POSTs a workspace write; a non-OK response throws a WriteError carrying the body text or the HTTP status. */
export async function postWorkspace<R>(
  route: string,
  body: WorkspaceRequest | HoldRequest,
): Promise<R> {
  const res = await apiFetch(`/v1/workspaces/${route}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
  return (await res.json()) as R
}
