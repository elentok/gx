import { useEffect, useState } from "react";

/** The visual viewport must be this much shorter than the layout viewport to count as a keyboard. */
const KEYBOARD_MIN_PX = 150;

/** While a text field has focus the viewport is re-read this often, in case iOS skips a resize event. */
const POLL_MS = 150;

const TEXT_INPUT_TYPES = new Set(["text", "search", "email", "url", "tel", "password", "number"]);

function acceptsText(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el instanceof HTMLTextAreaElement) return !el.readOnly && !el.disabled;
  if (el instanceof HTMLInputElement) return TEXT_INPUT_TYPES.has(el.type) && !el.readOnly && !el.disabled;
  return el.isContentEditable === true;
}

/**
 * True while an on-screen keyboard covers part of the window. iOS leaves `innerHeight` alone and shrinks only
 * the visual viewport, so the gap between the two is the keyboard. The visual viewport is measured in zoomed
 * CSS pixels, so its height is scaled back first: a pinch-zoom alone leaves no gap, while the zoom iOS applies
 * to a focused field (which must not hide the keyboard from us) still shows the keyboard's height. A gap alone
 * is not enough (browser chrome also opens one): a text field must have focus too. Pages use it to
 * drop chrome that would otherwise eat the little height the keyboard leaves. Always false without visualViewport.
 */
export function useKeyboardOpen(): boolean {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    // On focusout `activeElement` is already body, so a focus move is read from `relatedTarget` instead;
    // otherwise moving between two fields would flash closed.
    let timer: ReturnType<typeof setInterval> | undefined;
    const sync = (focused: EventTarget | null = document.activeElement) => {
      const editing = acceptsText(focused);
      setOpen(editing && window.innerHeight - vv.height * vv.scale >= KEYBOARD_MIN_PX);
      // iOS animates the keyboard in and sometimes never reports the final size (or reports it before focus),
      // so while a field is focused the size is also read on a timer instead of trusting the events alone.
      if (editing && timer === undefined) timer = setInterval(() => sync(), POLL_MS);
      if (!editing && timer !== undefined) {
        clearInterval(timer);
        timer = undefined;
      }
    };
    const onChange = () => sync();
    const onFocusOut = (e: FocusEvent) => sync(e.relatedTarget);
    sync();
    vv.addEventListener("resize", onChange);
    vv.addEventListener("scroll", onChange);
    window.addEventListener("resize", onChange);
    window.addEventListener("orientationchange", onChange);
    document.addEventListener("focusin", onChange);
    document.addEventListener("focusout", onFocusOut);
    return () => {
      clearInterval(timer);
      vv.removeEventListener("resize", onChange);
      vv.removeEventListener("scroll", onChange);
      window.removeEventListener("resize", onChange);
      window.removeEventListener("orientationchange", onChange);
      document.removeEventListener("focusin", onChange);
      document.removeEventListener("focusout", onFocusOut);
    };
  }, []);
  return open;
}
