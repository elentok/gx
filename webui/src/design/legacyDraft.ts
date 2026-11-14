import { hasText, type Fields } from "./draftFields"
import type { CreateDraft } from "./drafts"

const LEGACY_DRAFT_KEY = "gx.newDesign.draft"

/** Moves the pre-server localStorage draft into a draft of its own once. A key with no text is just removed; a failure keeps the key for the next press. */
export async function moveLegacyDraft(create: CreateDraft, workspace?: string) {
  try {
    const raw = localStorage.getItem(LEGACY_DRAFT_KEY)
    if (raw === null) return
    let legacy: Partial<Fields> = {}
    try {
      legacy = JSON.parse(raw) ?? {}
    } catch {
      // An unreadable key has no text.
    }
    if (hasText(legacy)) {
      await create({ workspace, project: legacy.project, slug: legacy.slug, idea: legacy.idea })
    }
    localStorage.removeItem(LEGACY_DRAFT_KEY)
  } catch {
    // Storage or the server is down; the key stays and the move retries.
  }
}
