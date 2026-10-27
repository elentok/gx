import { useCallback, useEffect, useRef, useState } from "react";
import { readPageCache, writePageCache } from "../pageCache";
import type { Answers } from "./round";

type RoundDraftData = { answers: Answers; notes: string; index: number; scroll: number };

const EMPTY: RoundDraftData = { answers: {}, notes: "", index: 0, scroll: 0 };

// Scrolls to `initial` on mount, then reports every scroll position. Pass a stable `onScroll`.
function useScrollRestore(initial: number, onScroll: (y: number) => void) {
  useEffect(() => {
    if (initial > 0) window.scrollTo(0, initial);
    const listener = () => onScroll(window.scrollY);
    window.addEventListener("scroll", listener, { passive: true });
    return () => window.removeEventListener("scroll", listener);
    // The restore is for the first render only; later drafts must not yank the page back.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [onScroll]);
}

/** What useRoundDraft returns: the draft and its setters. */
export type RoundDraft = ReturnType<typeof useRoundDraft>;

// A half-answered round survives a reload: its answers, the notes box, the open question and the page scroll.
// pageCache keeps one entry per session, tagged with the round, so a new round never picks up an old draft.
export function useRoundDraft(sessionId: string, roundKey: string) {
  const cacheKey = `round.${sessionId}`;
  const [draft, setDraft] = useState<RoundDraftData>(() => ({ ...EMPTY, ...readPageCache<RoundDraftData>(cacheKey, roundKey) }));
  // The scroll lives in a ref: every scroll event saving through state would repaint the page.
  const scroll = useRef(draft.scroll);
  const latest = useRef(draft);
  latest.current = draft;
  const sent = useRef(false);

  const save = useCallback(
    (d: RoundDraftData) => {
      if (!sent.current) writePageCache(cacheKey, { ...d, scroll: scroll.current }, roundKey);
    },
    [cacheKey, roundKey],
  );
  useEffect(() => save(draft), [save, draft]);
  useScrollRestore(
    draft.scroll,
    useCallback(
      (y: number) => {
        scroll.current = y;
        save(latest.current);
      },
      [save],
    ),
  );

  return {
    draft,
    setAnswers: useCallback((update: (all: Answers) => Answers) => setDraft((d) => ({ ...d, answers: update(d.answers) })), []),
    setNotes: useCallback((notes: string) => setDraft((d) => ({ ...d, notes })), []),
    setIndex: useCallback((index: number) => setDraft((d) => ({ ...d, index })), []),
    /** The round went out: drop the draft and stop saving until the round changes. */
    clear: useCallback(() => {
      sent.current = true;
      writePageCache(cacheKey, null, roundKey);
    }, [cacheKey, roundKey]),
  };
}
