import { useConnectionContext, useConnectionState } from "../connection"
import { isCurrent, useEpicListWhen } from "../epics/list"
import { isChat } from "../agents/sessionWrites"
import { useOpenChat, agentIdOf, chatDraftIdOf, draftIdOf } from "../shell/chords"
import type { EpicDetail, SessionInfo, TicketDetail } from "../generated/server"
import { readPageCache } from "../pageCache"
import { agentKindOf, chatEntity, viewDiffOf, type Entity } from "./entity"

type DraftRow = { member_key: string; label: string; workspace: string }

/** A draft page's entity. `row` is the snapshot's row, absent while the draft is new. */
export function draftEntity(
  row: DraftRow | undefined,
  kind: "design-draft" | "chat-draft",
  id: string,
): Entity {
  const chat = kind === "chat-draft"
  return {
    kind,
    key: row?.member_key ?? `${chat ? "chat-draft" : "draft"}/${id}`,
    name: row?.label ?? (chat ? "chat draft" : "draft"),
    surface: "page",
    workspace: row?.workspace,
  }
}

/** The entity the open page shows, with the path and open chat `useOpenChat` gives. Undefined off an entity page. */
export function useOpenEntity(): {
  path: string
  session: SessionInfo | undefined
  entity: Entity | undefined
} {
  const { path, session } = useOpenChat()
  const conn = useConnectionContext()
  const snapshot = useConnectionState(conn).snapshot
  const route = /^\/(e|d|t|archive)\/(.+)$/.exec(path)
  const kind = route?.[1]
  const key = route?.[2] ?? ""
  // Only an epic or a Design needs the list: its card says which workspace it sits in.
  const list = useEpicListWhen(kind === "e" || kind === "d", conn)
  const card = list?.epics?.find((r) => isCurrent(r, key))
  const base = { key, name: key, surface: "page" } as const

  // The page keeps its last detail in the page cache; the rules read it, never a fetch.
  if (kind === "e") {
    const detail = readPageCache<EpicDetail>("epic", key)
    return {
      path,
      session,
      entity: {
        ...base,
        kind: "epic",
        workspace: card?.workspace || undefined,
        viewDiff: detail ? viewDiffOf(key, detail) : undefined,
      },
    }
  }
  if (kind === "d")
    return {
      path,
      session,
      entity: { ...base, kind: "design", workspace: card?.workspace || undefined, session },
    }
  if (kind === "t") {
    const detail = readPageCache<TicketDetail>("ticket", key)
    return {
      path,
      session,
      entity: {
        ...base,
        kind: "ticket",
        status: detail?.status,
        hasWorktree: detail?.has_worktree,
        viewDiff: detail ? viewDiffOf(detail.epic, detail) : undefined,
      },
    }
  }
  if (kind === "archive") {
    const detail = readPageCache<EpicDetail>("archived-epic", key)
    return {
      path,
      session,
      entity: {
        ...base,
        kind: "epic",
        archived: true,
        viewDiff: detail ? viewDiffOf(key, detail) : undefined,
      },
    }
  }

  const chat = agentIdOf(path)
  if (chat) {
    if (!session) return { path, session, entity: undefined }
    const agent = agentKindOf(session)
    // A builder or flow session is never offered the plain chat's actions.
    const agentKind = agent === "chat" && !isChat(session) ? "managed" : agent
    return { path, session, entity: { ...chatEntity(session), surface: "page", agent: agentKind } }
  }
  const draft = draftIdOf(path)
  if (draft) {
    const row = snapshot?.design_drafts?.find((d) => d.id === draft)
    return { path, session, entity: draftEntity(row, "design-draft", draft) }
  }
  const chatDraft = chatDraftIdOf(path)
  if (chatDraft) {
    const row = snapshot?.chat_drafts?.find((d) => d.id === chatDraft)
    return { path, session, entity: draftEntity(row, "chat-draft", chatDraft) }
  }
  return { path, session, entity: undefined }
}
