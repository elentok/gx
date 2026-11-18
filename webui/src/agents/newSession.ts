import { apiFetch } from "../apiFetch"
import { postJson } from "../design/open"
import type { CreateSessionRequest, ProjectWorktrees, SessionResult } from "../generated/server"

/** The web create route takes only these fields of the full create request. */
export type NewSessionBody = Pick<
  CreateSessionRequest,
  "project" | "dir" | "worktree" | "name" | "prompt" | "tier" | "harness"
> & {
  /** The workspace the chat lands in; the web route reads it, the wire request has no such field. */
  workspace?: string
}
export type CreateSession = (body: NewSessionBody) => Promise<SessionResult>
export type FetchWorktrees = () => Promise<ProjectWorktrees[]>

export const postCreateSession: CreateSession = postJson("/v1/sessions/create")

/** The Default harness the Settings page shows: the global one and each project's override (empty when it follows the global). */
export interface DefaultHarnesses {
  default: string
  projects: { project: string; harness: string }[]
}
export type FetchDefaultHarnesses = () => Promise<DefaultHarnesses>

export const fetchDefaultHarnesses: FetchDefaultHarnesses = async () => {
  const res = await apiFetch("/v1/settings/agents")
  if (!res.ok) throw new Error(`default harness: HTTP ${res.status}`)
  return res.json()
}

export const fetchWorktrees: FetchWorktrees = async () => {
  const res = await apiFetch("/v1/projects/worktrees")
  if (!res.ok) throw new Error(`worktrees: HTTP ${res.status}`)
  return res.json()
}
