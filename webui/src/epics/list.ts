import { useCallback, useEffect, useState } from "react";
import type { Connection } from "../connection";
import type { EpicList } from "../generated/server";
import { readPageCache, writePageCache } from "../pageCache";
import { useRefetchOnEvents } from "../refetch";

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
  const [list, setList] = useState<EpicList | null>(() => readPageCache<EpicList>("epics"));
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);

  // A failed refetch keeps the last list: `failed` only shows while nothing is loaded.
  useEffect(() => {
    let live = true;
    setFailed(false);
    fetcher().then(
      (l) => {
        if (!live) return;
        setList(l);
        writePageCache("epics", l);
      },
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [fetcher, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  useRefetchOnEvents(conn, retry);

  return { list, failed: failed && list === null, retry };
}
