import { type DiffUrl, diffQuery } from "./diff/diffUrl"

export const epicHref = (address: string) => `#/e/${address}`

/** The Diff page; the query holds the project, refs, epic, narrowing and file. */
export const diffHref = (url: DiffUrl) => `#/diff?${diffQuery(url)}`

export const ticketPath = (address: string) => `/t/${address}`

export const ticketHref = (address: string) => `#${ticketPath(address)}`

export const answerPath = (address: string) => `/answer/${address}`

export const answerHref = (address: string) => `#${answerPath(address)}`

/** A Design is its map epic's canonical address (ADR 0027). */
export const designHref = (address: string) => `#/d/${address}`

/** A Design draft's page; the id is the draft's. */
export const draftHref = (id: string) => `#/drafts/${id}`

/** A chat draft's page; the id is the draft's. */
export const chatDraftHref = (id: string) => `#/chat-drafts/${id}`

/** A Triage is its project's, so the address is the project name. */
export const triageHref = (project: string) => `#/triage/${project}`

/** The epic address of the open page, or "". */
export function currentAddress(pathname: string): string {
  const [, kind, ...rest] = pathname.split("/")
  const id = decodeURIComponent(rest.join("/"))
  if (kind === "d" || kind === "e") return id
  if (kind === "t" || kind === "answer") return id.replace(/\/[^/]*$/, "")
  return ""
}

export const EPICS_HREF = "#/epics"

export const ARCHIVE_HREF = "#/archive"

/** An archived epic's read-only page; the address is the archive address. */
export const archiveHref = (address: string) => `${ARCHIVE_HREF}/${address}`

export const SCHEDULES_HREF = "#/schedules"

export const BACKLOG_HREF = "#/backlog"

export const INITIATIVES_HREF = "#/initiatives"

/** One Initiative's page; the address holds a colon, so the route takes a splat. */
export const initiativeHref = (address: string) => `${INITIATIVES_HREF}/${address}`

export const STATS_HREF = "#/stats"

export const CLEANUP_HREF = "#/cleanup"
export const TIPS_HREF = "#/tips"

export const AGENTS_HREF = "#/agents"

export const SETTINGS_HREF = "#/settings"

/** A session's label is its id. */
export const agentHref = (label: string) => `${AGENTS_HREF}/${label}`

/** A subagent's page: the one place its URL is built. */
export const subagentHref = (session: string, callId: string) =>
  `${agentHref(encodeURIComponent(session))}/subagents/${encodeURIComponent(callId)}`

/** The router path of an agent's page, for navigate(). */
export const agentPath = (label: string) => agentHref(label).slice(1)
