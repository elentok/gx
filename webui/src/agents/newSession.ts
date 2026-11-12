import { apiFetch } from "../apiFetch"
import { postJson } from "../design/open"
import type { CreateSessionRequest, ProjectWorktrees, SessionResult } from "../generated/server"

/** The web create route takes only these fields of the full create request. */
export type NewSessionBody = Pick<
  CreateSessionRequest,
  "project" | "dir" | "worktree" | "name" | "prompt" | "tier"
> & {
  /** The workspace the chat lands in; the web route reads it, the wire request has no such field. */
  workspace?: string
}
export type CreateSession = (body: NewSessionBody) => Promise<SessionResult>
export type FetchWorktrees = () => Promise<ProjectWorktrees[]>

export const postCreateSession: CreateSession = postJson("/v1/sessions/create")

export const fetchWorktrees: FetchWorktrees = async () => {
  const res = await apiFetch("/v1/projects/worktrees")
  if (!res.ok) throw new Error(`worktrees: HTTP ${res.status}`)
  return res.json()
}
