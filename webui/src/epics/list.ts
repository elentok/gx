import { apiFetch } from "../apiFetch"
import type { Connection } from "../connection"
import { SectionNeedsYou, type EpicList, type EpicListRow } from "../generated/server"
import { useFetched } from "../useFetched"

export type { EpicList }

export type FetchEpicList = () => Promise<EpicList>

export const fetchEpicList: FetchEpicList = async () => {
  const res = await apiFetch("/v1/epics")
  if (!res.ok) throw new Error(`epics: HTTP ${res.status}`)
  return res.json()
}

export interface EpicListState {
  list: EpicList | null
  /** The first fetch failed and nothing is on screen. */
  failed: boolean
  retry: () => void
}

// The page and the shell's sidebar both read the list: calls made while one is in flight share it, so they cost one request.
const inFlight = new WeakMap<FetchEpicList, Promise<EpicList>>()
const shareInFlight =
  (fetcher: FetchEpicList): FetchEpicList =>
  () => {
    let p = inFlight.get(fetcher)
    if (!p) {
      p = fetcher().finally(() => inFlight.delete(fetcher))
      inFlight.set(fetcher, p)
    }
    return p
  }

export function useEpicList(
  fetcher: FetchEpicList = fetchEpicList,
  conn?: Connection,
): EpicListState {
  const { data, failed, retry } = useFetched("epics", shareInFlight(fetcher), conn)
  return { list: data, failed, retry }
}

export const isCurrent = (row: EpicListRow, current: string) =>
  row.address === current || (row.addresses?.includes(current) ?? false)
