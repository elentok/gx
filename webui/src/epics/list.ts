import type { Connection } from "../connection";
import { SectionNeedsYou, type EpicList, type EpicListRow } from "../generated/server";
import { useFetched } from "../useFetched";

export type { EpicList };

export type FetchEpicList = () => Promise<EpicList>;

export const fetchEpicList: FetchEpicList = async () => {
  const res = await fetch("/v1/epics");
  if (!res.ok) throw new Error(`epics: HTTP ${res.status}`);
  return res.json();
};

export interface EpicListState {
  list: EpicList | null;
  /** The first fetch failed and nothing is on screen. */
  failed: boolean;
  retry: () => void;
}

export function useEpicList(fetcher: FetchEpicList = fetchEpicList, conn?: Connection): EpicListState {
  const { data, failed, retry } = useFetched("epics", fetcher, conn);
  return { list: data, failed, retry };
}

export const isCurrent = (row: EpicListRow, current: string) => row.address === current || (row.addresses?.includes(current) ?? false);

/** The Needs-you cards other than the epic on screen. */
export const waitingCount = (epics: EpicListRow[], current: string): number =>
  epics.filter((e) => e.section === SectionNeedsYou && !isCurrent(e, current)).length;
