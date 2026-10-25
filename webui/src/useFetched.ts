import { useCallback, useEffect, useRef, useState } from "react";
import type { Connection } from "./connection";
import { readPageCache, writePageCache } from "./pageCache";
import { useRefetchOnEvents } from "./refetch";

export interface FetchedState<T> {
  data: T | null;
  /** The first fetch failed and nothing is on screen. */
  failed: boolean;
  retry: () => void;
}

/**
 * Fetches on mount, whenever `id` changes and on server events, and keeps the last result in the page cache under `key`.
 * Data of another `id` is never shown: a changed `id` repaints from its own cache entry or returns null. A failed refetch keeps
 * the last data; `failed` only shows while nothing is loaded.
 */
export function useFetched<T>(key: string, fetcher: () => Promise<T>, conn?: Connection, id = "", debounceMs?: number): FetchedState<T> {
  const latest = useRef(fetcher);
  latest.current = fetcher;
  const [loaded, setLoaded] = useState<{ id: string; data: T } | null>(null);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let live = true;
    setFailed(false);
    latest.current().then(
      (data) => {
        if (!live) return;
        setLoaded({ id, data });
        writePageCache(key, data, id);
      },
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [key, id, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  useRefetchOnEvents(conn, retry, debounceMs);

  const data = loaded?.id === id ? loaded.data : readPageCache<T>(key, id);
  return { data, failed: failed && data === null, retry };
}
