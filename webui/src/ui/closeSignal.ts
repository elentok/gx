import { createContext, useContext, useEffect, useRef } from "react"
import { useLatest } from "../hooks"

/** A container (the sidebar) provides a value that changes whenever it closes; menus and popovers below it close on the change. */
export const CloseSignalContext = createContext<unknown>(undefined)

export const useCloseSignal = () => useContext(CloseSignalContext)

/** Calls `onClose` when `signal` changes, never on mount. */
export function useOnCloseSignal(signal: unknown, onClose: () => void) {
  const seen = useRef(signal)
  const latest = useLatest(onClose)
  useEffect(() => {
    if (Object.is(seen.current, signal)) return
    seen.current = signal
    latest.current()
  }, [signal])
}
