import { showToast } from "./ui/toaster";
import { WriteError } from "./useWrite";

// Same-origin, so the browser sends the Origin header the server's CSRF guard checks.
// The snapshot refetch that the server's afk-changed event triggers repaints the toggle.
export async function postAfk(on: boolean): Promise<void> {
  const res = await fetch("/v1/afk", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ on }),
  });
  if (!res.ok) throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`);
}

// Sets AFK to the opposite of what the last snapshot showed. A failed write leaves the toggle as it was.
export function toggleAfk(current: boolean | undefined): void {
  void postAfk(!current).catch((err) =>
    showToast({ type: "error", title: err instanceof WriteError ? err.message : "Could not reach gx. Try again." }),
  );
}
