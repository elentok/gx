import { useEffect, type RefObject } from "react"
import { hasLayer, useIsTopLayer } from "../ui/modal/layers"

/**
 * Escape and click-outside dismissal for a palette popover. Escape closes only while this palette is
 * the topmost layer; a mouse-down outside `root` closes it unless `phone` (full screen). Returns the
 * layer guard so the caller's open shortcut can stand down while another modal is open.
 */
export function useDismissLayer({
  open,
  close,
  root,
  phone,
}: {
  open: boolean
  close: () => void
  root: RefObject<HTMLElement | null>
  phone: boolean
}) {
  const isTop = useIsTopLayer(open)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isTop()) close()
    }
    const onDown = (e: MouseEvent) => {
      // A row's menu and its dialogs sit in portals outside the popover.
      if ((e.target as Element).closest?.('[role="menu"],[role="dialog"]')) return
      if (!phone && root.current && !root.current.contains(e.target as Node)) close()
    }
    window.addEventListener("keydown", onKey)
    document.addEventListener("mousedown", onDown)
    return () => {
      window.removeEventListener("keydown", onKey)
      document.removeEventListener("mousedown", onDown)
    }
  }, [phone, close, isTop, root])

  return { isTop, hasLayer }
}
