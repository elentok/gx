import { apiFetch } from "../apiFetch"
import type { InitiativeList } from "../generated/server"

export type FetchInitiatives = () => Promise<InitiativeList>

export const fetchInitiatives: FetchInitiatives = async () => {
  const res = await apiFetch("/v1/initiatives")
  if (!res.ok) throw new Error(`initiatives: HTTP ${res.status}`)
  return res.json()
}
