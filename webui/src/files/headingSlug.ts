/** GitHub-style slug of one heading's text, before the repeat suffix. */
export function slugOf(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\p{L}\p{N} _-]/gu, "")
    .replace(/ /g, "-")
}

/** One slug per heading, in document order; a repeat gets `-1`, `-2`, ... */
export function headingSlugs(texts: string[]): string[] {
  const seen = new Map<string, number>()
  return texts.map((text) => {
    const base = slugOf(text)
    const n = seen.get(base) ?? 0
    seen.set(base, n + 1)
    return n === 0 ? base : `${base}-${n}`
  })
}
