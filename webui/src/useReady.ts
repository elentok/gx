import { useEffect } from "react"
import { markReady, type DataSource, type Outcome } from "./routeSpans"
import type { FetchedState } from "./useFetched"

/**
 * A page calls this once its primary content, or its empty or error state, has painted. The first
 * call for a navigation wins; later calls do nothing.
 */
export function useReady(outcome: Outcome = "ready", dataSource: DataSource = "network"): void {
  useEffect(() => {
    markReady(outcome, dataSource)
  }, [outcome, dataSource])
}

/**
 * Ends the span from a page's primary `useFetched` state. Only pages call it, on their main fetch,
 * so a side fetch never ends a span. `empty` is the page's own "answer holds no rows" test.
 */
export function useFetchedReady(
  { failed, source }: Pick<FetchedState<unknown>, "failed" | "source">,
  empty = false,
): void {
  const settled = failed || source !== null
  const outcome: Outcome = failed ? "error" : empty ? "empty" : "ready"
  useEffect(() => {
    if (settled) markReady(outcome, source ?? "network")
  }, [settled, outcome, source])
}
