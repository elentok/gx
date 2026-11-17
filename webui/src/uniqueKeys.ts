/**
 * Pairs each item with a React key built from its content plus how many equal keys came before it.
 * For lists with no id, and rows that are never edited in place: removing a row moves no other
 * row's key, except among rows with identical content.
 */
export function withKeys<T>(items: readonly T[], contentOf: (item: T) => string) {
  const seen = new Map<string, number>()
  return items.map((item, index) => {
    const content = contentOf(item)
    const count = seen.get(content) ?? 0
    seen.set(content, count + 1)
    return { item, index, key: `${content}#${count}` }
  })
}
