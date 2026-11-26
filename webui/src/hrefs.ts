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

/** A subagent's page: the one place its URL is built. `origin` is the router path its back link returns to. */
export const subagentHref = (session: string, callId: string, origin?: string) => {
  const href = `${agentHref(encodeURIComponent(session))}/subagents/${encodeURIComponent(callId)}`
  return origin ? `${href}?from=${encodeURIComponent(origin)}` : href
}

/** The origin a subagent URL's query holds. */
export const subagentOrigin = (search: string) =>
  new URLSearchParams(search).get("from") ?? undefined

// The exact paths of the chat, Design and Triage pages. The origin comes from the URL, so nothing else is followed.
const ORIGIN_PATH = /^\/(agents|d|triage)\/[^/?#]+$/

/** Where a first-level subagent's back link goes: the origin, else the session's chat page. */
export const backHref = (session: string, origin?: string) =>
  origin && ORIGIN_PATH.test(origin) ? `#${origin}` : agentHref(session)

/** Splits the splat of `/agents/*` (already decoded by the router) into the session id and an optional subagent call id. */
export const parseAgentSplat = (splat: string): { session: string; callId?: string } => {
  const at = splat.indexOf("/subagents/")
  if (at < 0) return { session: splat }
  const callId = splat.slice(at + "/subagents/".length)
  return callId ? { session: splat.slice(0, at), callId } : { session: splat.slice(0, at) }
}

/** The router path of a hash href. */
export const routePath = (href: string) => href.slice(1)

/** The router path of an agent's page, for navigate(). */
export const agentPath = (label: string) => routePath(agentHref(label))
