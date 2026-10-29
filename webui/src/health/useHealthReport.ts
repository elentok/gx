import { useEffect, useState } from "react";
import type { HealthReport } from "../generated/server";
import { readPageCache, writePageCache } from "../pageCache";
import { fetchJson } from "../ticket/detail";

const POLL_MS = 5000;
const CACHE_KEY = "health";

// A report with the browser's clock at arrival, so the footer counts up without trusting that clock.
export interface Received {
  report: HealthReport;
  at: number;
}

/** The latest report, polled while `connected`; `null` until one has arrived. */
export function useHealthReport(connected: boolean): Received | null {
  // The cache survives a reload, so an offline reload still has values.
  const [received, setReceived] = useState<Received | null>(() => readPageCache<Received>(CACHE_KEY));

  // Offline stops the polling; reconnecting runs this again, which fetches at once.
  useEffect(() => {
    if (!connected) return;
    let live = true;
    // A "reading" CPU list gets one `wait` call, which the server answers when the list exists.
    const load = (wait = false): Promise<void> =>
      fetchJson<HealthReport>(wait ? "/v1/health?wait" : "/v1/health", "health")
        .then((res) => {
          if (!live || res.kind !== "ok") return;
          const next = { report: res.detail, at: Date.now() };
          writePageCache(CACHE_KEY, next);
          setReceived(next);
          if (!wait && res.detail.causes.cpu.marker === "reading") return load(true);
        })
        .catch(() => {});
    void load();
    const poll = setInterval(() => void load(), POLL_MS);
    return () => {
      live = false;
      clearInterval(poll);
    };
  }, [connected]);

  return received;
}
