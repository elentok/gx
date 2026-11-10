import type { Connection } from "../connection";
import type { EpicDetail, EpicDetailRow } from "../generated/server";
import { fetchJson, useDetail, type DetailResult, type DetailState } from "../ticket/detail";

export type { EpicDetail, EpicDetailRow };

export type FetchEpicDetail = (address: string) => Promise<DetailResult<EpicDetail>>;

export const fetchEpicDetail: FetchEpicDetail = (address) =>
  fetchJson(`/v1/epics/detail?address=${encodeURIComponent(address)}`, "epic detail");

export const fetchArchivedEpicDetail: FetchEpicDetail = (address) =>
  fetchJson(`/v1/epics/archived/detail?address=${encodeURIComponent(address)}`, "archived epic detail");

export function useEpicDetail(
  address: string,
  conn: Connection,
  fetcher?: FetchEpicDetail,
  archived = false,
): DetailState<EpicDetail> {
  // The archive address can equal an active one later, so the two pages keep separate caches.
  return useDetail(
    address,
    conn,
    fetcher ?? (archived ? fetchArchivedEpicDetail : fetchEpicDetail),
    `${address}/`,
    archived ? "archived-epic" : "epic",
  );
}
