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
