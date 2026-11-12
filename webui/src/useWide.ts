import { createContext, useContext, useEffect, useState } from "react"

/** Overrides the viewport width, so the gallery can show the phone layout on a wide page. */
export const WideContext = createContext<boolean | undefined>(undefined)

/** Phone below 640px, tablet from 640px, desktop from 992px (Chakra's `sm` and `lg`). */
export type Device = "phone" | "tablet" | "desktop"

/** Overrides the device type, so the gallery can show a tablet on a wide page. */
export const DeviceContext = createContext<Device | undefined>(undefined)

function useMedia(query: string, fallback: boolean) {
  const mq = () => (typeof window.matchMedia === "function" ? window.matchMedia(query) : undefined)
  const [matches, setMatches] = useState(() => mq()?.matches ?? fallback)
  useEffect(() => {
    const m = mq()
    if (!m) return
    const on = () => setMatches(m.matches)
    setMatches(m.matches)
    m.addEventListener("change", on)
    return () => m.removeEventListener("change", on)
  }, [query])
  return matches
}

/** Without matchMedia (tests) the device is desktop. */
export const useDevice = (): Device => {
  const forced = useContext(DeviceContext)
  const tablet = useMedia("(min-width: 40em)", true)
  const desktop = useMedia("(min-width: 62em)", true)
  return forced ?? (desktop ? "desktop" : tablet ? "tablet" : "phone")
}

export const useWide = () => {
  const forced = useContext(WideContext)
  const device = useDevice()
  return forced ?? device !== "phone"
}

/** Overrides the primary pointer, so the gallery and tests can show the touch layout with a mouse. */
export const CoarsePointerContext = createContext<boolean | undefined>(undefined)

/** True when the primary pointer is touch. Without matchMedia (tests) it is false. */
export const useCoarsePointer = () => {
  const forced = useContext(CoarsePointerContext)
  const coarse = useMedia("(pointer: coarse)", false)
  return forced ?? coarse
}
