import { useEffect, useState } from "react";

const SHEET_HEIGHT = 0.88;
const SHEET_TOP_GAP = 8;

/** The CSS variable the TopBar publishes on the document root. */
export const TOP_BAR_HEIGHT_VAR = "--top-bar-height";
const TOP_BAR = `var(${TOP_BAR_HEIGHT_VAR}, 0px)`;

const read = () => {
  const vv = window.visualViewport;
  return vv ? { top: vv.offsetTop ?? 0, height: vv.height } : undefined;
};

/**
 * The bottom sheet's overlay box and CSS height. iOS Safari does not shrink the layout viewport for the
 * on-screen keyboard, so a `position: fixed; inset: 0` overlay reaches under it. The overlay instead
 * covers the visual viewport, and the sheet follows its height to keep the footer visible. Without
 * visualViewport the overlay is the full screen and the sheet is 88%. `belowTopBar` leaves the TopBar
 * uncovered, using the height it publishes in `--top-bar-height`.
 */
export const useVisualViewportHeight = (belowTopBar = false) => {
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
  if (!vv) return { sheetHeight: "88%", overlay: { top: belowTopBar ? TOP_BAR : "0", left: "0", right: "0", bottom: "0" } };
  return {
    sheetHeight: `${Math.round(Math.min(SHEET_HEIGHT * window.innerHeight, vv.height - SHEET_TOP_GAP))}px`,
    overlay: belowTopBar
      ? { top: `calc(${vv.top}px + ${TOP_BAR})`, left: "0", right: "0", height: `calc(${vv.height}px - ${TOP_BAR})` }
      : { top: `${vv.top}px`, left: "0", right: "0", height: `${vv.height}px` },
  };
};
