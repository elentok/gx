import { apiFetch } from "../apiFetch";
import type { Connection } from "../connection";
import { StatsDefaultRange, StatsRangeMonth, StatsRangeQuarter, StatsRangeWeek, type Stats } from "../generated/server";
import { useFetched } from "../useFetched";

export type { Stats };

export const RANGES = [StatsRangeWeek, StatsRangeMonth, StatsRangeQuarter];
export const DEFAULT_RANGE = StatsDefaultRange;

// Stats move slowly, so a burst of events waits longer than on the Epics page.
export const STATS_REFETCH_DEBOUNCE_MS = 2000;

export interface StatsQuery {
  range: number;
  /** Empty means every project. */
  project: string;
}

export type FetchStats = (query: StatsQuery) => Promise<Stats>;

export const fetchStats: FetchStats = async ({ range, project }) => {
  const params = new URLSearchParams({ range: String(range) });
  if (project) params.set("project", project);
  const res = await apiFetch(`/v1/stats?${params}`);
  if (!res.ok) throw new Error(`stats: HTTP ${res.status}`);
  return res.json();
};

// A bad range falls back to the default instead of showing an error.
export const parseRange = (raw: string | null) => RANGES.find((r) => String(r) === raw) ?? DEFAULT_RANGE;

export interface StatsState {
  stats: Stats | null;
  /** The first fetch failed and nothing is on screen. */
  failed: boolean;
  retry: () => void;
}

export function useStats(query: StatsQuery, fetcher: FetchStats = fetchStats, conn?: Connection): StatsState {
  const { range, project } = query;
  const { data, failed, retry } = useFetched("stats", () => fetcher({ range, project }), conn, `${range}:${project}`, STATS_REFETCH_DEBOUNCE_MS);
  return { stats: data, failed, retry };
}
