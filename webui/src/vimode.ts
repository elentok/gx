import { apiFetch } from "./apiFetch"
import { useSnapshotField } from "./connection"
import { guarded } from "./ui/reportWriteFailure"
import { WriteError } from "./useWrite"

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
// The snapshot refetch that the server's vimode-changed event triggers repaints the toggle.
export async function postViMode(on: boolean): Promise<void> {
  const res = await apiFetch("/v1/vimode", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ on }),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
}

// Reads the key sequence (like "jk") that leaves insert mode; empty means off.
export function useViEscape(): string {
  return useSnapshotField((s) => s.vi_escape) ?? ""
}

// Sets Vi mode to the opposite of what the last snapshot showed. A failed write leaves the toggle as it was.
export function toggleViMode(current: boolean | undefined): void {
  guarded(postViMode(!current))
}

// Reads the setting anywhere in the app; no ConnectionContext reads as off.
export function useViMode(): boolean {
  return useSnapshotField((s) => s.vi_mode) ?? false
}
