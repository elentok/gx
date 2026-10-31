import { useContext, useSyncExternalStore } from "react";
import { apiFetch } from "./apiFetch";
import { ConnectionContext, type ConnectionState } from "./connection";
import { showToast } from "./ui/toaster";
import { WriteError } from "./useWrite";

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
// The snapshot refetch that the server's vimode-changed event triggers repaints the toggle.
export async function postViMode(on: boolean): Promise<void> {
  const res = await apiFetch("/v1/vimode", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ on }),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
}

// Sets Vi mode to the opposite of what the last snapshot showed. A failed write leaves the toggle as it was.
export function toggleViMode(current: boolean | undefined): void {
  void postViMode(!current).catch((err) =>
    showToast({ type: "error", title: err instanceof WriteError ? err.message : "Could not reach gx. Try again." }),
  );
}

const noopSubscribe = () => () => {};
const noState = (): ConnectionState | undefined => undefined;

// Reads the setting anywhere in the app. Many field tests render without a ConnectionContext, so a
// missing one reads as off instead of throwing like useConnectionContext does.
export function useViMode(): boolean {
  const conn = useContext(ConnectionContext);
  const state = useSyncExternalStore(conn?.subscribe ?? noopSubscribe, conn?.getState ?? noState);
  return state?.snapshot?.vi_mode ?? false;
}
