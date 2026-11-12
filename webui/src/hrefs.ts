export const epicHref = (address: string) => `#/e/${address}`

export const ticketPath = (address: string) => `/t/${address}`

export const ticketHref = (address: string) => `#${ticketPath(address)}`

/** A Design is its map epic's canonical address (ADR 0027). */
export const designHref = (address: string) => `#/d/${address}`

/** A Triage is its project's, so the address is the project name. */
export const triageHref = (project: string) => `#/triage/${project}`

/** The epic address of the open page, or "". */
export function currentAddress(pathname: string): string {
  const [, kind, ...rest] = pathname.split("/")
  const id = decodeURIComponent(rest.join("/"))
  if (kind === "d" || kind === "e") return id
  if (kind === "t") return id.replace(/\/[^/]*$/, "")
  return ""
}

export const EPICS_HREF = "#/epics"

export const ARCHIVE_HREF = "#/archive"

/** An archived epic's read-only page; the address is the archive address. */
export const archiveHref = (address: string) => `${ARCHIVE_HREF}/${address}`

export const SCHEDULES_HREF = "#/schedules"

export const STATS_HREF = "#/stats"

export const CLEANUP_HREF = "#/cleanup"
export const TIPS_HREF = "#/tips"

export const AGENTS_HREF = "#/agents"

export const SETTINGS_HREF = "#/settings"

/** A session's label is its id. */
export const agentHref = (label: string) => `${AGENTS_HREF}/${label}`

/** The router path of an agent's page, for navigate(). */
export const agentPath = (label: string) => agentHref(label).slice(1)
