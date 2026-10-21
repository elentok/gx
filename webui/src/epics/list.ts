import { useCallback, useEffect, useState } from "react";
import type { Connection } from "../connection";
import type { EpicList } from "../generated/server";

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

export const REFETCH_DEBOUNCE_MS = 500;

export function useEpicList(fetcher: FetchEpicList = fetchEpicList, conn?: Connection): EpicListState {
  const [list, setList] = useState<EpicList | null>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);

  // A failed refetch keeps the last list: `failed` only shows while nothing is loaded.
  useEffect(() => {
    let live = true;
    setFailed(false);
    fetcher().then(
      (l) => live && setList(l),
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [fetcher, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);

  useEffect(() => {
    if (!conn) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const off = conn.onEvent(() => {
      clearTimeout(timer);
      timer = setTimeout(retry, REFETCH_DEBOUNCE_MS);
    });
    return () => {
      off();
      clearTimeout(timer);
    };
  }, [conn, retry]);

  return { list, failed: failed && list === null, retry };
}
