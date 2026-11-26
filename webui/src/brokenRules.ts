import type { BrokenRuleReport } from "./generated/server"
import { pageUrl } from "./pageUrl"

// Plain fetch, not a shared helper: a signed-out page gets a 401 and must not redirect.
const ROUTE = "/v1/broken-rules"

const reported = new Set<string>()

/** Reports a broken web UI rule, once per rule and value for each page load. */
export function reportBrokenRule(rule: string, message: string, value: string): void {
  const key = JSON.stringify([rule, value])
  if (reported.has(key)) return
  reported.add(key)
  console.error(`${rule}: ${message}`)
  const body: BrokenRuleReport = {
    rule,
    message,
    value,
    url: pageUrl(),
    user_agent: navigator.userAgent,
  }
  fetch(ROUTE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }).catch(() => {})
}
