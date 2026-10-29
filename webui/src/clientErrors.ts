// Plain fetch, not a shared helper: it must work on the login page and never trigger a 401 redirect.
const ROUTE = "/v1/client-errors";

// A Setup code rides in the hash query (`#/setup?code=...`), so it never leaves the browser.
function pageUrl(): string {
  const { origin, pathname, search, hash } = window.location;
  const q = hash.indexOf("?");
  return origin + pathname + search + (q < 0 ? hash : hash.slice(0, q));
}

export function reportClientError(error: unknown): void {
  const err = error instanceof Error ? error : undefined;
  const body = {
    message: err ? err.message : String(error),
    stack: err?.stack ?? "",
    url: pageUrl(),
    user_agent: navigator.userAgent,
  };
  fetch(ROUTE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).catch(() => {});
}

export function installClientErrorReporting(): void {
  window.addEventListener("error", (e) => reportClientError(e.error ?? e.message));
  window.addEventListener("unhandledrejection", (e) => reportClientError(e.reason));
}
