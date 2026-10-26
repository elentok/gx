import { useEffect, useState } from "react";

const SHEET_HEIGHT = 0.88;
const SHEET_TOP_GAP = 8;

const read = () => {
  const vv = window.visualViewport;
  return vv ? { top: vv.offsetTop ?? 0, height: vv.height } : undefined;
};

/**
 * The bottom sheet's overlay box and CSS height. iOS Safari does not shrink the layout viewport for the
 * on-screen keyboard, so a `position: fixed; inset: 0` overlay reaches under it. The overlay instead
 * covers the visual viewport, and the sheet follows its height to keep the footer visible. Without
 * visualViewport the overlay is the full screen and the sheet is 88%.
 */
export const useVisualViewportHeight = () => {
  const [vv, setVv] = useState(read);
  useEffect(() => {
    const target = window.visualViewport;
    if (!target) return;
    const on = () => setVv(read());
    target.addEventListener("resize", on);
    target.addEventListener("scroll", on);
    return () => {
      target.removeEventListener("resize", on);
      target.removeEventListener("scroll", on);
    };
  }, []);
  if (!vv) return { sheetHeight: "88%", overlay: { inset: "0" } };
  return {
    sheetHeight: `${Math.round(Math.min(SHEET_HEIGHT * window.innerHeight, vv.height - SHEET_TOP_GAP))}px`,
    overlay: { top: `${vv.top}px`, left: "0", right: "0", height: `${vv.height}px` },
  };
};
