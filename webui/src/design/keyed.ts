/**
 * Pairs each item with a React key built from its own text plus how many equal texts came before it.
 * Rows that repeat stay distinct and a row keeps its key when others around it come and go.
 */
export function keyed<T>(
  items: readonly T[],
  text: (item: T) => string,
): { key: string; item: T }[] {
  const seen = new Map<string, number>()
  return items.map((item) => {
    const base = text(item)
    const n = seen.get(base) ?? 0
    seen.set(base, n + 1)
    return { key: `${base}#${n}`, item }
  })
}
