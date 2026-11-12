import { createContext, useContext, useEffect, useRef } from "react"

/** A container (the sidebar) provides a value that changes whenever it closes; menus and popovers below it close on the change. */
export const CloseSignalContext = createContext<unknown>(undefined)

export const useCloseSignal = () => useContext(CloseSignalContext)

/** Calls `onClose` when `signal` changes, never on mount. */
export function useOnCloseSignal(signal: unknown, onClose: () => void) {
  const first = useRef(true)
  const latest = useRef(onClose)
  latest.current = onClose
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    latest.current()
  }, [signal])
}
