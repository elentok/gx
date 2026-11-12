import { useContext, useSyncExternalStore } from "react"
import { postJSON } from "./auth/http"
import { ConnectionContext, type ConnectionState } from "./connection"
import type { DevRestartResult, DevStatus } from "./generated/server"
import { postSettings, GLOBAL_TARGET, setChange } from "./settings/settingsApi"
import { WriteError } from "./useWrite"

export const DEV_FRONTEND_KEY = "server.web.dev-frontend"

const NO_DEV: DevStatus = { state: "off", has_checkout: false }
const noopSubscribe = () => () => {}
const noState = (): ConnectionState | undefined => undefined

// Many field tests render without a ConnectionContext, so a missing one (or a missing block)
// reads as "no Dev checkout, off" instead of throwing.
export function useDev(): DevStatus {
  const conn = useContext(ConnectionContext)
  const state = useSyncExternalStore(conn?.subscribe ?? noopSubscribe, conn?.getState ?? noState)
  return state?.snapshot?.dev ?? NO_DEV
}

// A refusal throws a WriteError; the caller's guard toasts it.
// The dev-changed event the server emits triggers the snapshot refetch that repaints the UI.
export async function postDevFrontend(on: boolean): Promise<void> {
  const r = await postSettings(GLOBAL_TARGET, [setChange(DEV_FRONTEND_KEY, on)])
  if (r.refused) throw new WriteError(r.reason || "Could not change Dev mode.")
}

export async function postDevRestart(): Promise<void> {
  const r = await postJSON<DevRestartResult>("/v1/dev/restart")
  if (r.refused) throw new WriteError(r.message || "Vite is not down.")
}
