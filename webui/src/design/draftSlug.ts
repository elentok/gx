// Mirrors `drafts.SlugRe` in server/designs/drafts/store.go.
const SLUG_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/

/** Why `text` cannot be a draft's slug, or undefined when it can. An empty slug is valid: a draft may have none yet. */
export function slugProblem(text: string): string | undefined {
  if (text === "" || SLUG_RE.test(text)) return undefined
  return `${JSON.stringify(text)} is not a slug (want lowercase words joined by -)`
}
