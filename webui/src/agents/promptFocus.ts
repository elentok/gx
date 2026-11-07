import { useEffect, type RefObject } from "react";

const EVENT = "gx:focus-prompt";
let pending = false;

/**
 * Asks the chat page to focus its prompt. The request outlives the navigation that follows it: a composer that
 * mounts later takes it on mount, one already open takes it from the event. Any pointer press cancels it, so a
 * request nobody consumed cannot fire on a later click or tap.
 */
export function requestPromptFocus() {
  pending = true;
  window.dispatchEvent(new Event(EVENT));
}

if (typeof window !== "undefined") {
  window.addEventListener("pointerdown", () => (pending = false), true);
}

/** Focuses the textarea inside `container` when a request is pending, now or later. */
export function usePromptFocus(container: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const take = () => {
      if (!pending) return;
      const area = container.current?.querySelector("textarea");
      if (!area) return;
      pending = false;
      area.focus();
    };
    take();
    window.addEventListener(EVENT, take);
    return () => window.removeEventListener(EVENT, take);
  }, [container]);
}
