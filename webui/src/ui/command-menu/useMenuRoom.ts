import { useEffect, useState, type RefObject } from "react";

// The menu sits 6px off the field; the rest keeps it clear of the screen edge.
const OFFSET = 6;
const EDGE_GAP = 8;
// Below this the menu is a sliver nobody can use; a stale measurement must not shrink it that far.
const MIN_ROOM = 120;

/**
 * The room (px) the menu has between its field and the edge of the visual viewport, or undefined when
 * off or without visualViewport. iOS Safari does not shrink the layout viewport for the on-screen
 * keyboard, so the room is measured against the visual viewport. `menu` is the absolutely positioned
 * menu; its parent is the field's wrapper.
 */
export const useMenuRoom = (menu: RefObject<HTMLElement | null>, placement: "above" | "below", on: boolean) => {
  const [room, setRoom] = useState<number>();
  useEffect(() => {
    const vv = window.visualViewport;
    const field = menu.current?.parentElement;
    if (!on || !vv || !field) {
      setRoom(undefined);
      return;
    }
    const measure = () => {
      const r = field.getBoundingClientRect();
      const top = vv.offsetTop ?? 0;
      const space = placement === "above" ? r.top - top : top + vv.height - r.bottom;
      setRoom(Math.max(MIN_ROOM, Math.round(space - OFFSET - EDGE_GAP)));
    };
    measure();
    // Coming back from another app moves the viewport without a visualViewport event, so re-measure on these too.
    const later = () => requestAnimationFrame(measure);
    vv.addEventListener("resize", measure);
    vv.addEventListener("scroll", measure);
    window.addEventListener("resize", measure);
    window.addEventListener("pageshow", later);
    document.addEventListener("visibilitychange", later);
    document.addEventListener("focusin", later);
    return () => {
      vv.removeEventListener("resize", measure);
      vv.removeEventListener("scroll", measure);
      window.removeEventListener("resize", measure);
      window.removeEventListener("pageshow", later);
      document.removeEventListener("visibilitychange", later);
      document.removeEventListener("focusin", later);
    };
  }, [menu, placement, on]);
  return room;
};
