import { useEffect } from "react";
import type { Connection } from "./connection";

export const REFETCH_DEBOUNCE_MS = 500;

// Calls `refetch` once the server events settle, and on a reconnect.
export function useRefetchOnEvents(conn: Connection | undefined, refetch: () => void, debounceMs = REFETCH_DEBOUNCE_MS) {
  useEffect(() => {
    if (!conn) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const off = conn.onEvent(() => {
      clearTimeout(timer);
      timer = setTimeout(refetch, debounceMs);
    });
    // A reconnect takes a new snapshot without notifying event listeners, so watch `connected` itself.
    let wasConnected = conn.getState().connected;
    const offState = conn.subscribe(() => {
      const { connected } = conn.getState();
      if (connected && !wasConnected) refetch();
      wasConnected = connected;
    });
    return () => {
      off();
      offState();
      clearTimeout(timer);
    };
  }, [conn, refetch, debounceMs]);
}
