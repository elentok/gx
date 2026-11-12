import { useEffect, useRef } from "react"
import { hasLayer } from "../ui/modal/layers"

/** Horizontal travel that makes a gesture a swipe. */
export const SWIPE_MIN_PX = 40
/** A swipe starting within this distance of the left edge never opens the sidebar: the system's back swipe keeps it. */
export const EDGE_PX = 30
/** Surfaces that own horizontal swipes (a row with a single item has no overflow yet) carry this attribute. */
export const NO_SIDEBAR_SWIPE_ATTR = "data-no-sidebar-swipe"

/** Text-like fields and sliders keep their own horizontal drags. `select` is included: its picker takes the touch. */
const OWN_DRAG = `textarea, select, input[type=range], input:not([type=checkbox]):not([type=radio]):not([type=button]):not([type=submit])`

function isHorizontalScroller(el: Element): boolean {
  const overflowX = getComputedStyle(el).overflowX
  return (overflowX === "auto" || overflowX === "scroll") && el.scrollWidth > el.clientWidth
}

/** True when `el` or an ancestor is a horizontal scroller that can still move the way the finger is going. */
function scrollsHorizontally(el: Element | null, dx: number): boolean {
  for (; el && el !== document.body; el = el.parentElement) {
    if (!isHorizontalScroller(el)) continue
    // A finger moving left scrolls the content right.
    if (dx < 0 ? el.scrollLeft + el.clientWidth < el.scrollWidth : el.scrollLeft > 0) return true
  }
  return false
}

/** True when a touch starting at `el` belongs to something else: a horizontal scroller at any position, a marked surface or a field. */
function ownsHorizontalDrag(el: Element | null): boolean {
  if (el?.closest(`[${NO_SIDEBAR_SWIPE_ATTR}], ${OWN_DRAG}`)) return true
  for (; el && el !== document.body; el = el.parentElement)
    if (isHorizontalScroller(el)) return true
  return false
}

type Start = { x: number; y: number; target: Element | null }

/**
 * Swipe left on the open sliding sidebar (or its scrim) closes it; a rightward swipe from anywhere past the left edge
 * opens it. Does nothing while the sidebar is docked.
 */
export function useSidebarSwipe(opts: {
  docked: boolean
  open: boolean
  aside: React.RefObject<HTMLElement | null>
  onOpen: () => void
  onClose: () => void
}) {
  const latest = useRef(opts)
  latest.current = opts
  const { docked } = opts
  useEffect(() => {
    if (docked) return
    let start: Start | null = null
    const onStart = (e: TouchEvent) => {
      start =
        e.touches.length === 1
          ? { x: e.touches[0].clientX, y: e.touches[0].clientY, target: e.target as Element | null }
          : null
    }
    const onEnd = (e: TouchEvent) => {
      const s = start
      start = null
      const t = e.changedTouches[0]
      if (!s || !t) return
      const dx = t.clientX - s.x
      const dy = t.clientY - s.y
      if (Math.abs(dx) < SWIPE_MIN_PX || Math.abs(dx) < 2 * Math.abs(dy)) return
      const { open, aside, onOpen, onClose } = latest.current
      if (open) {
        if (scrollsHorizontally(s.target, dx)) return
        const inside =
          s.target &&
          (aside.current?.contains(s.target) ||
            s.target.closest('[data-testid="scrim"], [data-testid="menu-dim"]'))
        if (dx < 0 && inside) onClose()
      } else if (dx > 0 && s.x > EDGE_PX && !hasLayer() && !ownsHorizontalDrag(s.target)) {
        onOpen()
      }
    }
    document.addEventListener("touchstart", onStart, { passive: true })
    document.addEventListener("touchend", onEnd, { passive: true })
    return () => {
      document.removeEventListener("touchstart", onStart)
      document.removeEventListener("touchend", onEnd)
    }
  }, [docked])
}
