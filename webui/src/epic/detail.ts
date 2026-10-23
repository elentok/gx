import type { Connection } from "../connection";
import type { EpicDetail, EpicDetailRow } from "../generated/server";
import { fetchJson, useDetail, type DetailResult, type DetailState } from "../ticket/detail";

export type { EpicDetail, EpicDetailRow };

export type FetchEpicDetail = (address: string) => Promise<DetailResult<EpicDetail>>;

export const fetchEpicDetail: FetchEpicDetail = (address) =>
  fetchJson(`/v1/epics/detail?address=${encodeURIComponent(address)}`, "epic detail");

export function useEpicDetail(
  address: string,
  conn: Connection,
  fetcher: FetchEpicDetail = fetchEpicDetail,
): DetailState<EpicDetail> {
  return useDetail(address, conn, fetcher, `${address}/`, "epic");
}
