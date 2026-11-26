import { pageUrl } from "./pageUrl"

// Plain fetch, not a shared helper: it must work on the login page and never trigger a 401 redirect.
const ROUTE = "/v1/client-errors"

export function reportClientError(error: unknown): void {
  const err = error instanceof Error ? error : undefined
  const body = {
    message: err ? err.message : String(error),
    stack: err?.stack ?? "",
    url: pageUrl(),
    user_agent: navigator.userAgent,
  }
  fetch(ROUTE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).catch(() => {})
}

export function installClientErrorReporting(): void {
  window.addEventListener("error", (e) => reportClientError(e.error ?? e.message))
  window.addEventListener("unhandledrejection", (e) => reportClientError(e.reason))
}
