import { apiFetch } from "../apiFetch"
import type { Connection } from "../connection"
import type { Tip } from "../generated/server"
import { useFetched } from "../useFetched"

export type { Tip }

export type FetchTips = () => Promise<Tip[]>

export const fetchTips: FetchTips = async () => {
  // Reviewed: tips are page content, not a side fetch, so this stays on apiFetch.
  const res = await apiFetch("/v1/tips")
  if (!res.ok) throw new Error(`tips: HTTP ${res.status}`)
  return res.json()
}

export function useTips(fetcher: FetchTips = fetchTips, conn?: Connection) {
  const { data, failed, source, retry } = useFetched("tips", fetcher, conn)
  return { tips: data, failed, source, retry }
}
