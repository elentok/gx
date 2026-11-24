import type { Action, SessionInfo } from "../generated/server"

export type EntityKind = "epic" | "design" | "ticket" | "chat" | "design-draft" | "chat-draft"

/** What a chat entity is: a plain chat, or one of the agents the app runs itself ("managed": a builder or Triage session on a sidebar row, offered only Stop, Copy ID and Remove). */
export type AgentKind = "chat" | "queue" | "fix" | "investigate" | "managed"

/** Where the action starts. A Design's Stop reads it: the page shows one session, a row lists them all. */
export type Surface = "page" | "row"

/** One entity with the snapshot fields the registry rules read. The rules never fetch anything. */
export interface Entity {
  kind: EntityKind
  /** The address, the session id, or the draft's workspace membership key. */
  key: string
  name: string
  surface?: Surface
  workspace?: string
  held?: boolean
  queued?: boolean
  prUrl?: string
  archived?: boolean
  /** A chat's session, or the Design session its page shows. */
  session?: SessionInfo
  liveSessions?: number
  agent?: AgentKind
  /** A draft with no prompt, slug or images. Unknown (absent) counts as not empty. */
  draftEmpty?: boolean
  /** The ticket's server action list (`detail.actions`). */
  serverActions?: Action[]
  /** A ticket's status word as the page reads it; the address is `key`. */
  status?: string
}

/** The agent a session is: the Queue agent, a Fix agent, or a plain chat. */
export const agentKindOf = (s: SessionInfo): AgentKind =>
  s.queue ? "queue" : s.fix_of ? "fix" : "chat"

/** The chat entity of a session, as a row lists it. */
export const chatEntity = (s: SessionInfo): Entity => ({
  kind: "chat",
  key: s.id,
  name: s.name || s.id,
  surface: "row",
  session: s,
  agent: agentKindOf(s),
})

export const isChatEntity = (e: Entity) => e.kind === "chat"
export const isDraftEntity = (e: Entity) => e.kind === "design-draft" || e.kind === "chat-draft"
export const agentOf = (e: Entity): AgentKind => e.agent ?? "chat"
export const isPlainChat = (e: Entity) => isChatEntity(e) && agentOf(e) === "chat"
export const isLoopEntity = (e: Entity) => e.kind === "epic" || e.kind === "design"
