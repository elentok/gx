import { useEffect, useState } from "react";

/** The visual viewport's height, which shrinks while the on-screen keyboard is open. Undefined without visualViewport. */
export const useVisualViewportHeight = () => {
  const [height, setHeight] = useState(() => window.visualViewport?.height);
  useEffect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const on = () => setHeight(vv.height);
    vv.addEventListener("resize", on);
    return () => vv.removeEventListener("resize", on);
  }, []);
  return height;
};
