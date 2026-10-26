import { createContext, useContext, useEffect, useState } from "react";

/** Overrides the viewport width, so the gallery can show the phone layout on a wide page. */
export const WideContext = createContext<boolean | undefined>(undefined);

// Chakra's `lg` breakpoint. Without matchMedia (tests) the page is wide.
const WIDE = "(min-width: 62em)";
export const useWide = () => {
  const forced = useContext(WideContext);
  const mq = () => (typeof window.matchMedia === "function" ? window.matchMedia(WIDE) : undefined);
  const [wide, setWide] = useState(() => mq()?.matches ?? true);
  useEffect(() => {
    const m = mq();
    if (!m) return;
    const on = () => setWide(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return forced ?? wide;
};
