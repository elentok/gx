import { useCallback, useEffect, useState } from "react";
import type { Connection } from "../connection";
import type { Stats } from "../generated/server";
import { readPageCache, writePageCache } from "../pageCache";
import { useRefetchOnEvents } from "../refetch";

export type { Stats };

export const RANGES = [7, 30, 90] as const;
export const DEFAULT_RANGE = 30;

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
  const res = await fetch(`/v1/stats?${params}`);
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
  const id = `${range}:${project}`;
  const [loaded, setLoaded] = useState<{ id: string; stats: Stats } | null>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let live = true;
    setFailed(false);
    fetcher({ range, project }).then(
      (stats) => {
        if (!live) return;
        setLoaded({ id, stats });
        writePageCache("stats", stats, id);
      },
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [fetcher, range, project, id, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  useRefetchOnEvents(conn, retry, STATS_REFETCH_DEBOUNCE_MS);

  // Data of another filter is never shown: a changed filter repaints from its own cache entry or shows the spinner.
  const stats = loaded?.id === id ? loaded.stats : readPageCache<Stats>("stats", id);
  return { stats, failed: failed && stats === null, retry };
}
