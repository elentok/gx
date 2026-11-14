import { useEffect } from "react"
import { markReady, type DataSource, type Outcome } from "./routeSpans"
import type { DetailState } from "./ticket/detail"
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
  useReadyWhen(
    failed || source !== null,
    failed ? "error" : empty ? "empty" : "ready",
    source ?? "network",
  )
}

/** Ends the span once `settled`; for pages whose primary data is not a `useFetched` state. */
export function useReadyWhen(settled: boolean, outcome: Outcome, dataSource: DataSource): void {
  useEffect(() => {
    if (settled) markReady(outcome, dataSource)
  }, [settled, outcome, dataSource])
}

/**
 * Ends the span from a page's primary `useDetail` state: the first detail, or the not-found view.
 * `empty` is the page's own "nothing to show" test, such as a triage with no state.
 */
export function useDetailReady(
  { detail, notFound, source }: Pick<DetailState<unknown>, "detail" | "notFound" | "source">,
  empty = false,
): void {
  useReadyWhen(notFound || detail !== null, notFound || empty ? "empty" : "ready", source)
}
