import type { DesignDraft } from "../generated/server";
import type { SaveDraft } from "./drafts";

/** Text a failed autosave could not put on the server, kept per draft id until a save, Start or Discard settles it. */
export interface UnsavedEntry {
  revision: number;
  project: string;
  slug: string;
  idea: string;
  name?: string;
}

const KEY = "gx.newDesign.unsaved";

/** A draft the server never gave an id has no key of its own. */
export const NEW_DRAFT_KEY = "";

export function readUnsaved(): Record<string, UnsavedEntry> {
  try {
    const parsed = JSON.parse(localStorage.getItem(KEY) ?? "{}");
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function write(map: Record<string, UnsavedEntry>) {
  try {
    if (Object.keys(map).length) localStorage.setItem(KEY, JSON.stringify(map));
    else localStorage.removeItem(KEY);
  } catch {
    // Storage is a best-effort fallback.
  }
}

export function stashUnsaved(id: string, entry: UnsavedEntry) {
  write({ ...readUnsaved(), [id]: entry });
}

export function clearUnsaved(...ids: string[]) {
  const map = readUnsaved();
  for (const id of ids) delete map[id];
  write(map);
}

/** Why the form opened on a fallback entry instead of the latest draft. */
export type Restored = "unsaved" | "conflict" | "gone";

/**
 * Saves each fallback entry to its draft with its revision. The first one that cannot settle
 * (a refusal, or the server still down) is returned for the form to open on; its entry stays
 * until the form saves, starts or discards it.
 */
export async function saveUnsaved(save: SaveDraft): Promise<{ draft: DesignDraft; state: Restored } | undefined> {
  for (const [id, e] of Object.entries(readUnsaved())) {
    const fields = { project: e.project, slug: e.slug, idea: e.idea, name: e.name };
    let state: Restored;
    try {
      const res = await save({ id: id || undefined, ...fields, revision: e.revision });
      if (res.kind === "ok" || res.kind === "deleted") {
        clearUnsaved(id);
        continue;
      }
      state = res.kind;
    } catch {
      state = "unsaved";
    }
    return { draft: { id, images: [], edited_at: "", ...fields, revision: e.revision }, state };
  }
}
