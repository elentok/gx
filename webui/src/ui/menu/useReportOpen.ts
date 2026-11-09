import { useEffect, useRef } from "react";

/** Tells `onOpenChange` while a menu is open; unmounting an open menu reports it closed, so a holder never sticks. */
export function useReportOpen(open: boolean, onOpenChange?: (open: boolean) => void) {
  const report = useRef(onOpenChange);
  report.current = onOpenChange;
  useEffect(() => {
    if (!open) return;
    report.current?.(true);
    return () => report.current?.(false);
  }, [open]);
}
