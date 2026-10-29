import { useCallback, useEffect, useState } from "react";

// Per-session drafts live in localStorage so they survive a closed tab. Storage can throw
// (private mode, quota), so every read and write swallows errors, as pageCache does.
const COMPOSER = "gx.draft.composer.";
const ROUND = "gx.draft.round.";

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string | null): void {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Not worth failing the page over.
  }
}

/** The composer draft of one session and its setter. Empty text removes the stored draft. */
export function useComposerDraft(sessionId: string): [string, (text: string) => void] {
  const key = COMPOSER + sessionId;
  const [text, setText] = useState(() => read(key) ?? "");

  // The storage event only fires in other tabs, so a local edit never loops back here.
  useEffect(() => {
    setText(read(key) ?? "");
    const listener = (e: StorageEvent) => {
      if (e.key === key || e.key === null) setText(read(key) ?? "");
    };
    window.addEventListener("storage", listener);
    return () => window.removeEventListener("storage", listener);
  }, [key]);

  const set = useCallback(
    (next: string) => {
      setText(next);
      write(key, next === "" ? null : next);
    },
    [key],
  );
  return [text, set];
}

// One round draft per session, tagged with the round so a new round never picks up an old draft.
export function readRoundDraft<T>(sessionId: string, roundKey: string): T | null {
  try {
    const raw = read(ROUND + sessionId);
    const entry = raw ? (JSON.parse(raw) as { id: string; value: T }) : null;
    return entry && entry.id === roundKey ? entry.value : null;
  } catch {
    return null;
  }
}

export function writeRoundDraft(sessionId: string, roundKey: string, value: unknown): void {
  write(ROUND + sessionId, value === null ? null : JSON.stringify({ id: roundKey, value }));
}

/** Delete every stored draft whose session is not in `liveSessionIds`. */
export function pruneDrafts(liveSessionIds: Iterable<string>): void {
  try {
    const live = new Set(liveSessionIds);
    const stale: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      const prefix = key && [COMPOSER, ROUND].find((p) => key.startsWith(p));
      if (key && prefix && !live.has(key.slice(prefix.length))) stale.push(key);
    }
    stale.forEach((key) => localStorage.removeItem(key));
  } catch {
    // Not worth failing the page over.
  }
}
