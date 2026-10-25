import { useCallback, useEffect, useRef, useState } from "react";
import { readPageCache, writePageCache } from "../pageCache";
import type { Answers } from "./round";

export type RoundDraft = { answers: Answers; notes: string; index: number; scroll: number };

const EMPTY: RoundDraft = { answers: {}, notes: "", index: 0, scroll: 0 };

// A half-answered round survives a reload: its answers, the notes box, the open question and the page scroll.
// pageCache keeps one entry per session, tagged with the round, so a new round never picks up an old draft.
export function useRoundDraft(sessionId: string, roundKey: string) {
  const cacheKey = `round.${sessionId}`;
  const [draft, setDraft] = useState<RoundDraft>(() => ({ ...EMPTY, ...readPageCache<RoundDraft>(cacheKey, roundKey) }));
  // The scroll lives in a ref: every scroll event saving through state would repaint the page.
  const scroll = useRef(draft.scroll);
  const latest = useRef(draft);
  latest.current = draft;
  const sent = useRef(false);

  const save = useCallback(
    (d: RoundDraft) => {
      if (!sent.current) writePageCache(cacheKey, { ...d, scroll: scroll.current }, roundKey);
    },
    [cacheKey, roundKey],
  );
  useEffect(() => save(draft), [save, draft]);

  useEffect(() => {
    if (scroll.current > 0) window.scrollTo(0, scroll.current);
    const onScroll = () => {
      scroll.current = window.scrollY;
      save(latest.current);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, [save]);

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
