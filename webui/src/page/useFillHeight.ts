import { useEffect } from "react";

/** The CSS variable a fill page sizes itself by; it falls back to `100dvh` without visualViewport. */
export const FILL_HEIGHT_VAR = "--fill-height";

/**
 * iOS does not shrink the layout viewport for the on-screen keyboard, so a `100dvh` page stays taller than
 * what is visible and iOS pans the document up to reveal the focused field, carrying the header off screen.
 * While a fill page is mounted, publish the visual viewport's height and put the document back at the top
 * whenever the viewport pans, so the page is always exactly as tall as what the person can see.
 */
export function useFillHeight(active: boolean | undefined) {
  useEffect(() => {
    const vv = window.visualViewport;
    if (!active || !vv) return;
    const root = document.documentElement;
    const sync = () => {
      root.style.setProperty(FILL_HEIGHT_VAR, `${Math.round(vv.height)}px`);
      if (vv.offsetTop > 0 || window.scrollY > 0) window.scrollTo(0, 0);
    };
    sync();
    vv.addEventListener("resize", sync);
    vv.addEventListener("scroll", sync);
    return () => {
      vv.removeEventListener("resize", sync);
      vv.removeEventListener("scroll", sync);
      root.style.removeProperty(FILL_HEIGHT_VAR);
    };
  }, [active]);
}
