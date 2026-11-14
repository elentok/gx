import { useEffect } from "react"
import { markReady, type DataSource, type Outcome } from "./routeSpans"

/**
 * A page calls this once its primary content, or its empty or error state, has painted. The first
 * call for a navigation wins; later calls do nothing.
 */
export function useReady(outcome: Outcome = "ready", dataSource: DataSource = "network"): void {
  useEffect(() => {
    markReady(outcome, dataSource)
  }, [outcome, dataSource])
}
