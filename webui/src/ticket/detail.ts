import { useCallback, useEffect, useRef, useState } from "react";
import type { Connection } from "../connection";
import type { TicketDetail } from "../generated/server";
import { readPageCache, writePageCache } from "../pageCache";

export type { TicketDetail };

export type DetailResult<T = TicketDetail> = { kind: "ok"; detail: T } | { kind: "not-found" };

export type FetchDetail = (address: string) => Promise<DetailResult>;

export async function fetchJson<T>(url: string, label: string): Promise<DetailResult<T>> {
  const res = await fetch(url);
  if (res.status === 404 || res.status === 400) return { kind: "not-found" };
  if (!res.ok) throw new Error(`${label}: HTTP ${res.status}`);
  return { kind: "ok", detail: await res.json() };
}

export const fetchDetail: FetchDetail = (address) =>
  fetchJson(`/v1/tickets/detail?address=${encodeURIComponent(address)}`, "detail");

export interface DetailState<T = TicketDetail> {
  detail: T | null;
  notFound: boolean;
  /** Re-reads the detail; call it after each POST. */
  refetch: () => void;
}

// A ticket address is <project>:<epic>/<id>; the page refetches for any ticket of its epic.
export const epicOfTicket = (address: string) => address.slice(0, Math.max(0, address.lastIndexOf("/")));

export function useTicketDetail(address: string, conn: Connection, fetcher: FetchDetail = fetchDetail): DetailState {
  return useDetail(address, conn, fetcher, `${epicOfTicket(address)}/`, "ticket");
}

// A failed fetch keeps the last detail on screen: the Offline strip says why it may be stale.
export function useDetail<T>(
  address: string,
  conn: Connection,
  fetcher: (address: string) => Promise<DetailResult<T>>,
  // The page also goes stale on an event whose address starts with this: an epic's tickets, a ticket's epic-mates.
  watchPrefix?: string,
  // Names the sessionStorage entry that repaints this page after a reload.
  cacheKey?: string,
): DetailState<T> {
  const [state, setState] = useState<{ address: string; detail: T | null; notFound: boolean }>(() => ({
    address,
    detail: cacheKey ? readPageCache<T>(cacheKey, address) : null,
    notFound: false,
  }));
  const latest = useRef(0);

  const refetch = useCallback(() => {
    const n = ++latest.current;
    fetcher(address).then(
      (r) => {
        if (n !== latest.current) return;
        if (cacheKey && r.kind === "ok") writePageCache(cacheKey, r.detail, address);
        setState(
          r.kind === "ok"
            ? { address, detail: r.detail, notFound: false }
            : { address, detail: null, notFound: true },
        );
      },
      () => {},
    );
  }, [address, fetcher, cacheKey]);

  useEffect(() => {
    refetch();
    const off = conn.onEvent((a) => {
      if (a === address || (watchPrefix && a?.startsWith(watchPrefix))) refetch();
    });
    return () => {
      off();
      latest.current++;
    };
  }, [address, conn, refetch, watchPrefix]);

  // Another ticket's detail must never show under this address while it loads.
  const mine = state.address === address;
  return { detail: mine ? state.detail : null, notFound: mine && state.notFound, refetch };
}
