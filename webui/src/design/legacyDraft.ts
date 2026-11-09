import { hasText, type Fields } from "./draftFields";
import type { SaveDraft } from "./drafts";

const LEGACY_DRAFT_KEY = "gx.newDesign.draft";

/** Moves the pre-server localStorage draft to a server draft once. A refusal or failure keeps the key for the next open. */
export async function moveLegacyDraft(save: SaveDraft) {
  let legacy: Partial<Fields> = {};
  try {
    const raw = localStorage.getItem(LEGACY_DRAFT_KEY);
    if (raw === null) return;
    try {
      legacy = JSON.parse(raw) ?? {};
    } catch {
      // An unreadable key has no text.
    }
    if (hasText(legacy)) {
      const res = await save({ project: legacy.project ?? "", slug: legacy.slug ?? "", idea: legacy.idea ?? "", revision: 0 });
      if (res.kind !== "ok") return;
    }
    localStorage.removeItem(LEGACY_DRAFT_KEY);
  } catch {
    // Storage or the server is down; the key stays and the move retries.
  }
}
