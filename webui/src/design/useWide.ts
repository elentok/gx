import { useEffect, useState } from "react";

// Chakra's `lg` breakpoint. Without matchMedia (tests) the page is wide.
const WIDE = "(min-width: 62em)";
export const useWide = () => {
  const mq = () => (typeof window.matchMedia === "function" ? window.matchMedia(WIDE) : undefined);
  const [wide, setWide] = useState(() => mq()?.matches ?? true);
  useEffect(() => {
    const m = mq();
    if (!m) return;
    const on = () => setWide(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return wide;
};
