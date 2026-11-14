import { apiFetch } from "../apiFetch"
import { postJson } from "../design/open"
import type {
  BacklogList,
  FinishMoveRequest,
  FinishMoveResult,
  SessionResult,
  StartDesignRequest,
} from "../generated/server"

export type FetchBacklog = () => Promise<BacklogList>

export const fetchBacklog: FetchBacklog = async () => {
  const res = await apiFetch("/v1/backlog")
  if (!res.ok) throw new Error(`backlog: HTTP ${res.status}`)
  return res.json()
}

export type PostStartDesign = (body: StartDesignRequest) => Promise<SessionResult>
export type PostFinishMove = (body: FinishMoveRequest) => Promise<FinishMoveResult>

export const postStartDesign: PostStartDesign = postJson("/v1/backlog/start-design")
export const postFinishMove: PostFinishMove = postJson<FinishMoveResult>("/v1/backlog/finish-move")
