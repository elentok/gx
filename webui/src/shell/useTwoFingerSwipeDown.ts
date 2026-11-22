import { useEffect } from "react"

/** The swipe must start inside the centered part of the window: this share of its width and height. */
const CENTER_SHARE = 0.75
/** How far the two fingers must travel down, in px. */
const MIN_TRAVEL = 90
/** Sideways drift allowed, as a share of the downward travel. */
const MAX_DRIFT = 0.6
/** How much the gap between the fingers may change before it counts as a pinch, in px. */
const MAX_PINCH = 40

interface Pt {
  x: number
  y: number
}

const centroid = (a: Pt, b: Pt): Pt => ({ x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 })
const gap = (a: Pt, b: Pt) => Math.hypot(a.x - b.x, a.y - b.y)
const pt = (t: Touch): Pt => ({ x: t.clientX, y: t.clientY })

function inCenter(p: Pt): boolean {
  const mx = (window.innerWidth * (1 - CENTER_SHARE)) / 2
  const my = (window.innerHeight * (1 - CENTER_SHARE)) / 2
  return p.x >= mx && p.x <= window.innerWidth - mx && p.y >= my && p.y <= window.innerHeight - my
}

/**
 * Calls `onSwipe` once per two-finger swipe down that starts in the center 75% of the window.
 * Touch events only fire on touch screens, so this covers phones and tablets.
 */
export function useTwoFingerSwipeDown(onSwipe: () => void) {
  useEffect(() => {
    let start: { c: Pt; gap: number } | null = null
    let fired = false

    const onStart = (e: TouchEvent) => {
      if (e.touches.length !== 2) {
        start = null
        return
      }
      const a = pt(e.touches[0]!)
      const b = pt(e.touches[1]!)
      start = inCenter(a) && inCenter(b) ? { c: centroid(a, b), gap: gap(a, b) } : null
      fired = false
    }

    const onMove = (e: TouchEvent) => {
      if (!start || fired || e.touches.length !== 2) return
      const a = pt(e.touches[0]!)
      const b = pt(e.touches[1]!)
      if (Math.abs(gap(a, b) - start.gap) > MAX_PINCH) {
        start = null
        return
      }
      const c = centroid(a, b)
      const dy = c.y - start.c.y
      const dx = Math.abs(c.x - start.c.x)
      if (dy < -MIN_TRAVEL || dx > MIN_TRAVEL * 2) {
        start = null
        return
      }
      if (dy >= MIN_TRAVEL && dx <= dy * MAX_DRIFT) {
        fired = true
        start = null
        e.preventDefault()
        onSwipe()
      }
    }

    const onEnd = () => {
      start = null
    }

    document.addEventListener("touchstart", onStart, { passive: true })
    document.addEventListener("touchmove", onMove, { passive: false })
    document.addEventListener("touchend", onEnd, { passive: true })
    document.addEventListener("touchcancel", onEnd, { passive: true })
    return () => {
      document.removeEventListener("touchstart", onStart)
      document.removeEventListener("touchmove", onMove)
      document.removeEventListener("touchend", onEnd)
      document.removeEventListener("touchcancel", onEnd)
    }
  }, [onSwipe])
}
