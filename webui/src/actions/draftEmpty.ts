import { realChatDrafts, type ChatDraftsClient } from "../design/chatDrafts"
import { readDraft as fetchDraft, type ReadDraft } from "../design/drafts"

export interface DraftReaders {
  readDraft: ReadDraft
  readChatDraft: ChatDraftsClient["read"]
}

const realReaders: DraftReaders = { readDraft: fetchDraft, readChatDraft: realChatDrafts.read }

/** Whether a draft has nothing to lose. A draft that cannot be read counts as not empty. */
export async function isDraftEmpty(
  kind: "design-draft" | "chat-draft",
  id: string,
  read: DraftReaders = realReaders,
): Promise<boolean> {
  try {
    if (kind === "chat-draft") {
      const res = await read.readChatDraft(id)
      return res.kind === "ok" && !res.draft.brief.trim() && res.draft.images.length === 0
    }
    const res = await read.readDraft(id)
    const d = res.kind === "ok" ? res.draft : undefined
    return !!d && !d.idea.trim() && !d.slug && d.images.length === 0
  } catch {
    return false
  }
}
