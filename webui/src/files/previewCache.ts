const cache = new Map<string, unknown>()

const key = (path: string, modified: string | undefined) => `${path}\0${modified ?? ""}`

/**
 * Previews for the page's life, keyed by canonical path and mtime: a changed file has a new mtime,
 * so it never serves a stale preview. Text and media bodies store what they fetched here.
 */
export const previewCache = {
  get<T>(path: string, modified?: string): T | undefined {
    return cache.get(key(path, modified)) as T | undefined
  },
  set(path: string, modified: string | undefined, value: unknown) {
    cache.set(key(path, modified), value)
  },
  clear() {
    cache.clear()
  },
}
