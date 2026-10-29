import { useCallback, useEffect, useRef, useState } from "react";
import { readRoundDraft, writeRoundDraft } from "../drafts";
import type { Answers } from "./round";

type RoundDraftData = { answers: Answers; index: number; scroll: number };

const EMPTY: RoundDraftData = { answers: {}, index: 0, scroll: 0 };

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

// A half-answered round survives a reload: its answers, the open question and the page scroll.
// drafts.ts keeps one entry per session, tagged with the round, so a new round never picks up an old draft.
export function useRoundDraft(sessionId: string, roundKey: string) {
  const [draft, setDraft] = useState<RoundDraftData>(() => ({ ...EMPTY, ...readRoundDraft<RoundDraftData>(sessionId, roundKey) }));
  // The scroll lives in a ref: every scroll event saving through state would repaint the page.
  const scroll = useRef(draft.scroll);
  const latest = useRef(draft);
  latest.current = draft;
  const sent = useRef(false);

  const save = useCallback(
    (d: RoundDraftData) => {
      if (!sent.current) writeRoundDraft(sessionId, roundKey, { ...d, scroll: scroll.current });
    },
    [sessionId, roundKey],
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
    setIndex: useCallback((index: number) => setDraft((d) => ({ ...d, index })), []),
    /** The round went out: drop the draft and stop saving until the round changes. */
    clear: useCallback(() => {
      sent.current = true;
      writeRoundDraft(sessionId, roundKey, null);
    }, [sessionId, roundKey]),
  };
}
