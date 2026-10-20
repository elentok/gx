import { useCallback, useEffect, useRef, useState } from "react";
import type { Connection } from "../connection";
import type { TicketDetail } from "../generated/server";

export type { TicketDetail };

export type DetailResult = { kind: "ok"; detail: TicketDetail } | { kind: "not-found" };

export type FetchDetail = (address: string) => Promise<DetailResult>;

export const fetchDetail: FetchDetail = async (address) => {
  const res = await fetch(`/v1/tickets/detail?address=${encodeURIComponent(address)}`);
  if (res.status === 404) return { kind: "not-found" };
  if (!res.ok) throw new Error(`detail: HTTP ${res.status}`);
  return { kind: "ok", detail: await res.json() };
};

export interface DetailState {
  detail: TicketDetail | null;
  notFound: boolean;
  /** Re-reads the detail; call it after each POST. */
  refetch: () => void;
}

// A failed fetch keeps the last detail on screen: the Offline strip says why it may be stale.
export function useTicketDetail(address: string, conn: Connection, fetcher: FetchDetail = fetchDetail): DetailState {
  const [state, setState] = useState<{ address: string; detail: TicketDetail | null; notFound: boolean }>({
    address,
    detail: null,
    notFound: false,
  });
  const latest = useRef(0);

  const refetch = useCallback(() => {
    const n = ++latest.current;
    fetcher(address).then(
      (r) => {
        if (n !== latest.current) return;
        setState(
          r.kind === "ok"
            ? { address, detail: r.detail, notFound: false }
            : { address, detail: null, notFound: true },
        );
      },
      () => {},
    );
  }, [address, fetcher]);

  useEffect(() => {
    refetch();
    const off = conn.onEvent((a) => {
      if (a === address) refetch();
    });
    return () => {
      off();
      latest.current++;
    };
  }, [address, conn, refetch]);

  // Another ticket's detail must never show under this address while it loads.
  const mine = state.address === address;
  return { detail: mine ? state.detail : null, notFound: mine && state.notFound, refetch };
}
