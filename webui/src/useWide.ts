import { createContext, useContext, useEffect, useState } from "react";

/** Overrides the viewport width, so the gallery can show the phone layout on a wide page. */
export const WideContext = createContext<boolean | undefined>(undefined);

/** Narrow pages are one column and go wide at 640px. Wide pages have a sidebar and go wide at 992px. */
export type PageMode = "narrow" | "wide";
export const PageModeContext = createContext<PageMode>("wide");

// Chakra's `sm` and `lg` breakpoints. Without matchMedia (tests) the page is wide.
const QUERY: Record<PageMode, string> = {
  narrow: "(min-width: 40em)",
  wide: "(min-width: 62em)",
};
export const useWide = () => {
  const forced = useContext(WideContext);
  const query = QUERY[useContext(PageModeContext)];
  const mq = () => (typeof window.matchMedia === "function" ? window.matchMedia(query) : undefined);
  const [wide, setWide] = useState(() => mq()?.matches ?? true);
  useEffect(() => {
    const m = mq();
    if (!m) return;
    const on = () => setWide(m.matches);
    setWide(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, [query]);
  return forced ?? wide;
};
