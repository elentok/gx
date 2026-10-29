// Each page keeps its last server data in sessionStorage, so a page iOS Safari dropped in the
// background repaints at once after a reload. The fresh fetch replaces the copy within moments.
// One entry per key keeps the storage small: a detail page stores only the address it showed last.
// Storage can throw (private mode, quota), so both calls swallow errors.
export function readPageCache<T>(key: string, id = ""): T | null {
  try {
    const raw = sessionStorage.getItem(`gx.page.${key}`);
    const entry = raw ? (JSON.parse(raw) as { id: string; value: T }) : null;
    return entry && entry.id === id ? entry.value : null;
  } catch {
    return null;
  }
}

export function clearPageCache(): void {
  try {
    for (const key of Object.keys(sessionStorage)) {
      if (key.startsWith("gx.page.")) sessionStorage.removeItem(key);
    }
  } catch {
    // Not worth failing the page over.
  }
}

export function writePageCache(key: string, value: unknown, id = ""): void {
  try {
    sessionStorage.setItem(`gx.page.${key}`, JSON.stringify({ id, value }));
  } catch {
    // Not worth failing the page over.
  }
}
