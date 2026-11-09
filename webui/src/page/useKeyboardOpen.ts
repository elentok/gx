import { useEffect, useState } from "react";

/** The visual viewport must be this much shorter than the layout viewport to count as a keyboard. */
const KEYBOARD_MIN_PX = 150;

const TEXT_INPUT_TYPES = new Set(["text", "search", "email", "url", "tel", "password", "number"]);

function acceptsText(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  if (el instanceof HTMLTextAreaElement) return !el.readOnly && !el.disabled;
  if (el instanceof HTMLInputElement) return TEXT_INPUT_TYPES.has(el.type) && !el.readOnly && !el.disabled;
  return el.isContentEditable === true;
}

/**
 * True while an on-screen keyboard covers part of the window. iOS leaves `innerHeight` alone and shrinks only
 * the visual viewport, so the gap between the two is the keyboard. A gap alone is not enough (pinch-zoom and
 * browser chrome also open one): a text field must have focus and the page must not be zoomed. Pages use it to
 * drop chrome that would otherwise eat the little height the keyboard leaves. Always false without visualViewport.
 */
export function useKeyboardOpen(): boolean {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    // On focusout `activeElement` is already body, so a focus move is read from `relatedTarget` instead;
    // otherwise moving between two fields would flash closed.
    const sync = (focused: EventTarget | null = document.activeElement) =>
      setOpen(vv.scale <= 1 && window.innerHeight - vv.height >= KEYBOARD_MIN_PX && acceptsText(focused));
    const onResize = () => sync();
    const onFocusIn = () => sync();
    const onFocusOut = (e: FocusEvent) => sync(e.relatedTarget);
    sync();
    vv.addEventListener("resize", onResize);
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("focusout", onFocusOut);
    return () => {
      vv.removeEventListener("resize", onResize);
      document.removeEventListener("focusin", onFocusIn);
      document.removeEventListener("focusout", onFocusOut);
    };
  }, []);
  return open;
}
