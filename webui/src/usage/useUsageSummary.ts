import { useEffect, useState } from "react";
import type { Connection } from "../connection";
import { EventBudgetChanged, EventLimitsChanged, type UsageSummary } from "../generated/server";
import { fetchJson } from "../ticket/detail";

export type FetchSummary = () => Promise<UsageSummary>;

export const REFRESH_MS = 60_000;

// A bad status and not-found both count as a failed fetch.
export const fetchSummary: FetchSummary = async () => {
  const r = await fetchJson<UsageSummary>("/v1/usage/summary", "usage summary");
  if (r.kind !== "ok") throw new Error("usage summary: not found");
  return r.detail;
};

export interface UsageSummaryState {
  summary: UsageSummary;
  /** Date.now() at the moment the summary arrived. */
  now: number;
}

// Not useFetched: a failed refresh must clear the data, and nothing is cached across reloads.
// Returns null until the first fetch succeeds and again after any failed refresh.
export function useUsageSummary(conn: Connection, fetcher: FetchSummary = fetchSummary): UsageSummaryState | null {
  const [state, setState] = useState<UsageSummaryState | null>(null);

  useEffect(() => {
    let latest = 0;
    const refetch = () => {
      const n = ++latest;
      fetcher().then(
        (summary) => {
          if (n === latest) setState({ summary, now: Date.now() });
        },
        () => {
          if (n === latest) setState(null);
        },
      );
    };
    refetch();
    const timer = setInterval(refetch, REFRESH_MS);
    const off = conn.onEvent((_address, type) => {
      if (type === EventLimitsChanged || type === EventBudgetChanged) refetch();
    });
    return () => {
      clearInterval(timer);
      off();
      latest++;
    };
  }, [conn, fetcher]);

  return state;
}
