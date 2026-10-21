import { useCallback, useEffect, useState } from "react";
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

export function useEpicList(fetcher: FetchEpicList = fetchEpicList): EpicListState {
  const [list, setList] = useState<EpicList | null>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);

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
  return { list, failed: failed && list === null, retry };
}
