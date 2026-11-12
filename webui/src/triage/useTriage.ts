import type { Connection } from "../connection"
import type { TriageView } from "../generated/server"
import { fetchJson, useDetail, type DetailResult, type DetailState } from "../ticket/detail"

export type FetchTriage = (project: string) => Promise<DetailResult<TriageView>>

export const fetchTriage: FetchTriage = (project) =>
  fetchJson(`/v1/triage?project=${encodeURIComponent(project)}`, "triage")

/** The project's Triage view; `triage-changed` events carry the project as their address, which useDetail already watches. */
export function useTriage(
  project: string,
  conn: Connection,
  fetcher: FetchTriage = fetchTriage,
): DetailState<TriageView> {
  return useDetail(project, conn, fetcher, undefined, "triage")
}
