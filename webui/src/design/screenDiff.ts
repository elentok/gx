/** Indexes of the rows in `next` that differ from the row at the same index in `prev`; added rows count as changed. */
export function changedRows(prev: string[], next: string[]): number[] {
  const out: number[] = []
  next.forEach((row, i) => {
    if (prev[i] !== row) out.push(i)
  })
  return out
}

/** True when both screens have the same rows. A removed row changes nothing in `next` to mark, so length counts too. */
export const sameScreen = (prev: string[], next: string[]) =>
  prev.length === next.length && changedRows(prev, next).length === 0
