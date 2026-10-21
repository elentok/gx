import type { Connection } from "../connection";
import type { EpicDetail, EpicDetailRow } from "../generated/server";
import { useDetail, type DetailState } from "../ticket/detail";

export type { EpicDetail, EpicDetailRow };

export type EpicDetailResult = { kind: "ok"; detail: EpicDetail } | { kind: "not-found" };

export type FetchEpicDetail = (address: string) => Promise<EpicDetailResult>;

export const fetchEpicDetail: FetchEpicDetail = async (address) => {
  const res = await fetch(`/v1/epics/detail?address=${encodeURIComponent(address)}`);
  if (res.status === 404) return { kind: "not-found" };
  if (!res.ok) throw new Error(`epic detail: HTTP ${res.status}`);
  return { kind: "ok", detail: await res.json() };
};

export function useEpicDetail(
  address: string,
  conn: Connection,
  fetcher: FetchEpicDetail = fetchEpicDetail,
): DetailState<EpicDetail> {
  return useDetail(address, conn, fetcher, true);
}
