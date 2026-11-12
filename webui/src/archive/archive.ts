import { apiFetch } from "../apiFetch"
import type { ArchivedEpicList } from "../generated/server"

export type { ArchivedEpicList }

export type FetchArchived = () => Promise<ArchivedEpicList>

export const fetchArchived: FetchArchived = async () => {
  const res = await apiFetch("/v1/epics/archived")
  if (!res.ok) throw new Error(`archived: HTTP ${res.status}`)
  return res.json()
}
