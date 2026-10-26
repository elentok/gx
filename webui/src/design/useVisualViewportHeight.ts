import { useEffect, useState } from "react";

const SHEET_HEIGHT = 0.88;
const SHEET_TOP_GAP = 8;

/** The bottom sheet's CSS height. The on-screen keyboard shrinks the visual viewport, so the sheet follows it to keep the footer visible; without visualViewport it is 88%. */
export const useVisualViewportHeight = () => {
  const [height, setHeight] = useState(() => window.visualViewport?.height);
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const on = () => setHeight(vv.height);
    vv.addEventListener("resize", on);
    return () => vv.removeEventListener("resize", on);
  }, []);
  return height ? `${Math.round(Math.min(SHEET_HEIGHT * window.innerHeight, height - SHEET_TOP_GAP))}px` : "88%";
};
