// A Setup code rides in the hash query (`#/setup?code=...`), so it never leaves the browser.
export function pageUrl(): string {
  const { origin, pathname, search, hash } = window.location
  const q = hash.indexOf("?")
  return origin + pathname + search + (q < 0 ? hash : hash.slice(0, q))
}
