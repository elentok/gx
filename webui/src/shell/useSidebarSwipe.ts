import { useEffect, useRef } from "react";

/** Horizontal travel that makes a gesture a swipe. */
export const SWIPE_MIN_PX = 40;
/** A swipe starting this close to the left edge may open the sidebar. */
export const EDGE_PX = 24;

/** Whether the page runs as a home-screen app, where the edge swipe is ours (in a Safari tab it is back). */
export function isStandalone(): boolean {
  if ((navigator as Navigator & { standalone?: boolean }).standalone) return true;
  return typeof matchMedia === "function" && matchMedia("(display-mode: standalone)").matches;
}

/** True when `el` or an ancestor is a horizontal scroller that can still move the way the finger is going. */
function scrollsHorizontally(el: Element | null, dx: number): boolean {
  for (; el && el !== document.body; el = el.parentElement) {
    const overflowX = getComputedStyle(el).overflowX;
    if (overflowX !== "auto" && overflowX !== "scroll") continue;
    if (el.scrollWidth <= el.clientWidth) continue;
    // A finger moving left scrolls the content right.
    if (dx < 0 ? el.scrollLeft + el.clientWidth < el.scrollWidth : el.scrollLeft > 0) return true;
  }
  return false;
}

type Start = { x: number; y: number; target: Element | null };

/**
 * Swipe left on the open sliding sidebar (or its scrim) closes it; swipe right from the left edge opens it, in a
 * home-screen app only. Does nothing while the sidebar is docked.
 */
export function useSidebarSwipe(opts: {
  docked: boolean;
  open: boolean;
  aside: React.RefObject<HTMLElement | null>;
  onOpen: () => void;
  onClose: () => void;
}) {
  const latest = useRef(opts);
  latest.current = opts;
  const { docked } = opts;
  useEffect(() => {
    if (docked) return;
    let start: Start | null = null;
    const onStart = (e: TouchEvent) => {
      start = e.touches.length === 1 ? { x: e.touches[0].clientX, y: e.touches[0].clientY, target: e.target as Element | null } : null;
    };
    const onEnd = (e: TouchEvent) => {
      const s = start;
      start = null;
      const t = e.changedTouches[0];
      if (!s || !t) return;
      const dx = t.clientX - s.x;
      const dy = t.clientY - s.y;
      if (Math.abs(dx) < SWIPE_MIN_PX || Math.abs(dx) < 2 * Math.abs(dy)) return;
      if (scrollsHorizontally(s.target, dx)) return;
      const { open, aside, onOpen, onClose } = latest.current;
      if (open) {
        const inside = s.target && (aside.current?.contains(s.target) || s.target.closest('[data-testid="scrim"]'));
        if (dx < 0 && inside) onClose();
      } else if (dx > 0 && s.x <= EDGE_PX && isStandalone()) {
        onOpen();
      }
    };
    document.addEventListener("touchstart", onStart, { passive: true });
    document.addEventListener("touchend", onEnd, { passive: true });
    return () => {
      document.removeEventListener("touchstart", onStart);
      document.removeEventListener("touchend", onEnd);
    };
  }, [docked]);
}
