import { useEffect, useState } from "react";

/** The visual viewport must be this much shorter than the layout viewport to count as a keyboard. */
const KEYBOARD_MIN_PX = 150;

/**
 * True while an on-screen keyboard covers part of the window. iOS leaves `innerHeight` alone and shrinks only
 * the visual viewport, so the gap between the two is the keyboard. Pages use it to drop chrome that would
 * otherwise eat the little height the keyboard leaves. Always false without visualViewport.
 */
export function useKeyboardOpen(): boolean {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const sync = () => setOpen(window.innerHeight - vv.height > KEYBOARD_MIN_PX);
    sync();
    vv.addEventListener("resize", sync);
    return () => vv.removeEventListener("resize", sync);
  }, []);
  return open;
}
