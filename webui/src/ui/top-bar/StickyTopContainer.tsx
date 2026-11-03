import { Box } from "@chakra-ui/react";
import { type ReactNode, useEffect, useRef } from "react";
import { TOP_BAR_HEIGHT_VAR } from "../modal/useVisualViewportHeight";

// Sticks to the top of the page and holds the banners above the top bar, so they move as one.
// It spans the page container's padding, so banners reach the window edges.
export function StickyTopContainer({ ruled = false, children }: { ruled?: boolean; children: ReactNode }) {
  const box = useRef<HTMLDivElement>(null);
  // iOS leaves the layout viewport in place when the keyboard opens and pans the visual viewport, which
  // carries a sticky element off screen. Follow the visual viewport's offset to keep it in view.
  useEffect(() => {
    const el = box.current;
    const vv = window.visualViewport;
    if (!el || !vv) return;
    const follow = () => {
      el.style.transform = vv.offsetTop > 0 ? `translateY(${vv.offsetTop}px)` : "";
    };
    follow();
    vv.addEventListener("scroll", follow);
    vv.addEventListener("resize", follow);
    return () => {
      vv.removeEventListener("scroll", follow);
      vv.removeEventListener("resize", follow);
    };
  }, []);
  // Sheets that sit below the container read its real height from the root.
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const publish = () => document.documentElement.style.setProperty(TOP_BAR_HEIGHT_VAR, `${el.offsetHeight}px`);
    publish();
    if (typeof ResizeObserver === "undefined") return () => document.documentElement.style.removeProperty(TOP_BAR_HEIGHT_VAR);
    const observer = new ResizeObserver(publish);
    observer.observe(el);
    return () => {
      observer.disconnect();
      document.documentElement.style.removeProperty(TOP_BAR_HEIGHT_VAR);
    };
  }, []);
  return (
    <Box ref={box} mx="-4" position="sticky" top="0" zIndex="sticky" bg="surface" {...(ruled && { borderBottomWidth: "1px", borderColor: "border" })}>
      {children}
    </Box>
  );
}
