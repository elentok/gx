import type { DesignDraft } from "../generated/server"
import type { SaveDraft } from "./drafts"

/** The prompt a failed autosave could not put on the server, kept per draft id until a save, Start or Remove settles it. */
export interface UnsavedEntry {
  revision: number
  idea: string
}

const KEY = "gx.newDesign.unsaved"

export function readUnsaved(): Record<string, UnsavedEntry> {
  try {
    const parsed = JSON.parse(localStorage.getItem(KEY) ?? "{}")
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}

function write(map: Record<string, UnsavedEntry>) {
  try {
    if (Object.keys(map).length) localStorage.setItem(KEY, JSON.stringify(map))
    else localStorage.removeItem(KEY)
  } catch {
    // Storage is a best-effort fallback.
  }
}

export function stashUnsaved(id: string, entry: UnsavedEntry) {
  write({ ...readUnsaved(), [id]: entry })
}

export function clearUnsaved(...ids: string[]) {
  const map = readUnsaved()
  for (const id of ids) delete map[id]
  write(map)
}

/** Why the page opened on a fallback entry instead of the server's copy. */
export type Restored = "unsaved" | "conflict"

/**
 * Saves each fallback entry to its draft with its revision. The first one that cannot settle
 * (a refusal, or the server still down) is returned for the page to open on when it is that
 * draft's page; its entry stays until the page saves, starts or removes it.
 */
export async function saveUnsaved(
  save: SaveDraft,
): Promise<{ draft: DesignDraft; state: Restored } | undefined> {
  for (const [id, e] of Object.entries(readUnsaved())) {
    // An entry from before drafts always had ids has no draft to go back to.
    if (!id) {
      clearUnsaved(id)
      continue
    }
    let state: Restored
    try {
      const res = await save({ id, idea: e.idea, revision: e.revision })
      // A draft that is gone has nothing to keep the text for.
      if (res.kind === "ok" || res.kind === "deleted" || res.kind === "gone") {
        clearUnsaved(id)
        continue
      }
      state = "conflict"
    } catch {
      state = "unsaved"
    }
    return {
      draft: {
        id,
        images: [],
        edited_at: "",
        label: "",
        project: "",
        slug: "",
        idea: e.idea,
        revision: e.revision,
      },
      state,
    }
  }
}
