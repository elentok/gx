import { useContext, useSyncExternalStore } from "react";
import { postJSON } from "./auth/http";
import { ConnectionContext, type ConnectionState } from "./connection";
import type { DevRestartResult, DevStatus } from "./generated/server";
import { postSettings, GLOBAL_TARGET, setChange } from "./settings/settingsApi";
import { showToast } from "./ui/toaster";
import { WriteError } from "./useWrite";

export const DEV_FRONTEND_KEY = "server.web.dev-frontend";

const NO_DEV: DevStatus = { state: "off", has_checkout: false };
const noopSubscribe = () => () => {};
const noState = (): ConnectionState | undefined => undefined;

// Many field tests render without a ConnectionContext, so a missing one (or a missing block)
// reads as "no Dev checkout, off" instead of throwing.
export function useDev(): DevStatus {
  const conn = useContext(ConnectionContext);
  const state = useSyncExternalStore(conn?.subscribe ?? noopSubscribe, conn?.getState ?? noState);
  return state?.snapshot?.dev ?? NO_DEV;
}

function toastFailure(err: unknown) {
  showToast({ type: "error", title: err instanceof WriteError ? err.message : "Could not reach gx. Try again." });
}

// The dev-changed event the server emits triggers the snapshot refetch that repaints the UI.
export function postDevFrontend(on: boolean): void {
  void postSettings(GLOBAL_TARGET, [setChange(DEV_FRONTEND_KEY, on)])
    .then((r) => {
      if (r.refused) showToast({ type: "error", title: r.reason || "Could not change Dev mode." });
    })
    .catch(toastFailure);
}

export function postDevRestart(): void {
  void postJSON<DevRestartResult>("/v1/dev/restart")
    .then((r) => {
      if (r.refused) showToast({ type: "error", title: r.message || "Vite is not down." });
    })
    .catch(toastFailure);
}
