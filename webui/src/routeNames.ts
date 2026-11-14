// The only route names a sample may carry. Keyed by the router's route pattern, never by the URL,
// so no ticket address, query or hash text can leak.
const SETTINGS = "settings"

export const ROUTE_NAMES: Record<string, string> = {
  "/epics": "epics",
  "/archive": "archive",
  "/archive/*": "archive-epic",
  "/agents/*": "agents",
  "/files/*": "files",
  "/stats": "stats",
  "/cleanup": "cleanup",
  "/schedules": "schedules",
  "/schedules/new": "schedule",
  "/schedules/*": "schedule",
  "/tips": "tips",
  "/settings": SETTINGS,
  "/settings/s/:section": SETTINGS,
  "/settings/p/:project": SETTINGS,
  "/settings/p/:project/s/:section": SETTINGS,
  "/settings/logins": SETTINGS,
  "/settings/logins/:id": SETTINGS,
  "/settings/password": SETTINGS,
  "/settings/authenticator": SETTINGS,
  "/settings/certificate": SETTINGS,
  "/settings/agents": SETTINGS,
  "/settings/*": SETTINGS,
  "/d/*": "design",
  "/drafts/:id": "design-draft",
  "/triage/*": "triage",
  "/t/*": "ticket",
  "/e/*": "epic",
  "/diff": "diff",
}

export function routeName(pattern: string | undefined): string | undefined {
  return pattern === undefined ? undefined : ROUTE_NAMES[pattern]
}
