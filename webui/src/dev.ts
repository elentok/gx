import { postJSON } from "./auth/http"
import { useSnapshotField } from "./connection"
import type { DevRestartResult, DevStatus } from "./generated/server"
import { postSettings, GLOBAL_TARGET, setChange } from "./settings/settingsApi"
import { WriteError } from "./useWrite"

export const DEV_FRONTEND_KEY = "server.web.dev-frontend"

const NO_DEV: DevStatus = { state: "off", has_checkout: false }
// A missing ConnectionContext (or block) reads as "no Dev checkout, off".
export function useDev(): DevStatus {
  return useSnapshotField((s) => s.dev) ?? NO_DEV
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
