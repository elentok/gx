import type { SessionCommandEntry } from "../generated/server";

export type TokenKind = "/" | "$";

/** The word being completed: text.slice(start, end) is the whole token, query is what was typed after the sigil up to the cursor. */
export interface OpenToken {
  kind: TokenKind;
  start: number;
  end: number;
  query: string;
}

export interface TokenContext {
  harness: string | undefined;
  disabled?: boolean;
}

const isSpace = (c: string) => /\s/.test(c);

function wordBounds(text: string, cursor: number): [number, number] {
  let start = cursor;
  while (start > 0 && !isSpace(text[start - 1])) start--;
  let end = cursor;
  while (end < text.length && !isSpace(text[end])) end++;
  return [start, end];
}

/** `/` opens only as the first character of a draft; `$` opens on any word, in Codex only. Neither survives whitespace in the word. */
export function openToken(text: string, cursor: number, ctx: TokenContext): OpenToken | null {
  if (ctx.disabled) return null;
  const [start, end] = wordBounds(text, cursor);
  const word = text.slice(start, end);
  if (word.length === 0 || cursor <= start) return null;
  const sigil = word[0];
  const kind: TokenKind | null = sigil === "/" && start === 0 ? "/" : sigil === "$" && ctx.harness === "codex" ? "$" : null;
  if (!kind) return null;
  return { kind, start, end, query: text.slice(start + 1, cursor) };
}

export interface Segment {
  text: string;
  match: boolean;
}

export interface Match {
  entry: SessionCommandEntry;
  /** The name or alias that matched; the segments describe this string. */
  matched: string;
  segments: Segment[];
}

function segmentsOf(name: string, at: number, len: number): Segment[] {
  const parts: Segment[] = [
    { text: name.slice(0, at), match: false },
    { text: name.slice(at, at + len), match: true },
    { text: name.slice(at + len), match: false },
  ];
  return parts.filter((p) => p.text !== "");
}

/** Prefix matches first, then substring matches, alphabetical by name in each group. Names win over aliases. */
export function matchCommands(entries: SessionCommandEntry[], query: string): Match[] {
  const q = query.toLowerCase();
  const prefix: Match[] = [];
  const inside: Match[] = [];
  for (const entry of entries) {
    const candidates = [entry.name, ...(entry.aliases ?? [])];
    let found: { matched: string; at: number } | undefined;
    for (const rank of [0, 1]) {
      for (const c of candidates) {
        const at = c.toLowerCase().indexOf(q);
        if (at < 0 || (rank === 0 && at !== 0)) continue;
        found = { matched: c, at };
        break;
      }
      if (found) {
        const m = { entry, matched: found.matched, segments: segmentsOf(found.matched, found.at, q.length) };
        (rank === 0 ? prefix : inside).push(m);
        break;
      }
    }
  }
  const byName = (a: Match, b: Match) => a.entry.name.localeCompare(b.entry.name);
  return [...prefix.sort(byName), ...inside.sort(byName)];
}

/** Replaces the token with `<sigil>name ` and puts the cursor after the space. Never sends. */
export function fillCommand(text: string, token: OpenToken, name: string): { text: string; cursor: number } {
  const fill = `${token.kind}${name} `;
  // The fill ends in a space already: don't double the one that followed the token.
  const rest = text.slice(token.end).replace(/^ /, "");
  return { text: text.slice(0, token.start) + fill + rest, cursor: token.start + fill.length };
}
