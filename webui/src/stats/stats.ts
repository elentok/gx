import { useRef } from "react"
import { apiFetch } from "../apiFetch"
import type { Connection } from "../connection"
import {
  StatsDefaultRange,
  StatsRangeMonth,
  StatsRangeQuarter,
  StatsRangeWeek,
  type Stats,
} from "../generated/server"
import { useFetched, type FetchedState } from "../useFetched"

export type { Stats }

export const RANGES = [StatsRangeWeek, StatsRangeMonth, StatsRangeQuarter]
export const DEFAULT_RANGE = StatsDefaultRange

// Stats move slowly, so a burst of events waits longer than on the Epics page.
export const STATS_REFETCH_DEBOUNCE_MS = 2000

export interface StatsQuery {
  range: number
  /** Empty means every project. */
  project: string
  /** Web performance counts dev samples too. */
  includeDev: boolean
}

export type FetchStats = (query: StatsQuery) => Promise<Stats>

export const fetchStats: FetchStats = async ({ range, project, includeDev }) => {
  const params = new URLSearchParams({ range: String(range) })
  if (project) params.set("project", project)
  if (includeDev) params.set("include_dev", "true")
  const res = await apiFetch(`/v1/stats?${params}`)
  if (!res.ok) throw new Error(`stats: HTTP ${res.status}`)
  return res.json()
}

// A bad range falls back to the default instead of showing an error.
export const parseRange = (raw: string | null) =>
  RANGES.find((r) => String(r) === raw) ?? DEFAULT_RANGE

export interface StatsState {
  stats: Stats | null
  /** The first fetch failed and nothing is on screen. */
  failed: boolean
  /** `stats` are from an earlier query and the current one is still loading. */
  stale: boolean
  /** `stats` are from an earlier query and the current one failed. */
  refetchFailed: boolean
  source: FetchedState<Stats>["source"]
  retry: () => void
}

export function useStats(
  query: StatsQuery,
  fetcher: FetchStats = fetchStats,
  conn?: Connection,
): StatsState {
  const { range, project, includeDev } = query
  const { data, failed, source, retry } = useFetched(
    "stats",
    () => fetcher({ range, project, includeDev }),
    conn,
    {
      id: `${range}:${project}:${includeDev}`,
      debounceMs: STATS_REFETCH_DEBOUNCE_MS,
    },
  )
  // useFetched never shows another id's data, so a filter change reads as
  // "no data". Keep the last loaded stats here instead of changing that contract.
  const last = useRef<Stats | null>(null)
  if (data) last.current = data
  const stats = data ?? last.current
  // useFetched reports `failed` while its own data is null, which includes a failed filter change.
  const refetchFailed = failed && !data && !!stats
  return {
    stats,
    failed: failed && !stats,
    stale: !data && !!stats && !failed,
    refetchFailed,
    source,
    retry,
  }
}
