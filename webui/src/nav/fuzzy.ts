/**
 * Fuzzy match: the query's characters must appear in the text in order, case-insensitively.
 * Returns a score where lower is better (the tightest span of the text the query covers,
 * then the earliest start), or null when the text does not match.
 */
export function fuzzyScore(text: string, query: string): number | null {
  const t = text.toLowerCase();
  const q = query.toLowerCase();
  if (q === "") return 0;
  let best: number | null = null;
  for (let start = t.indexOf(q[0]); start !== -1; start = t.indexOf(q[0], start + 1)) {
    let at = start;
    let ok = true;
    for (let i = 1; i < q.length; i++) {
      at = t.indexOf(q[i], at + 1);
      if (at === -1) {
        ok = false;
        break;
      }
    }
    if (!ok) break; // a later start cannot match either
    const score = (at - start) * 1000 + start;
    if (best === null || score < best) best = score;
  }
  return best;
}
