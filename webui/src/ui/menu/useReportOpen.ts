import { useEffect } from "react"
import { useLatest } from "../../hooks"

/** Tells `onOpenChange` while a menu is open; unmounting an open menu reports it closed, so a holder never sticks. */
export function useReportOpen(open: boolean, onOpenChange?: (open: boolean) => void) {
  const report = useLatest(onOpenChange)
  useEffect(() => {
    if (!open) return
    report.current?.(true)
    return () => report.current?.(false)
  }, [open])
}
