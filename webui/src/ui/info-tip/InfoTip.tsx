import { chakra } from "@chakra-ui/react"
import { useEffect, useRef, type ReactNode } from "react"
import { TipPopup } from "./TipPopup"

interface Props {
  /** The accessible name of the trigger. */
  label: string
  /** The tooltip text. */
  content: ReactNode
  /** The trigger face. It defaults to the "i" marker. */
  children?: ReactNode
  /** "marker" is the 14px circle; "chip" is a text pill that takes its colour from `chipColor`. */
  variant?: "marker" | "chip"
  /** The chip's border and text colour (a Chakra token). */
  chipColor?: string
  /** Draw the chip's border. It defaults to true; false keeps the chip's size and drops the line. */
  outline?: boolean
}

const markerStyle = {
  w: "14px",
  h: "14px",
  borderRadius: "full",
  borderColor: "text.muted",
  color: "text.muted",
  fontSize: "10px",
  fontStyle: "italic",
} as const

export function InfoTip({
  label,
  content,
  children = "i",
  variant = "marker",
  chipColor = "text.muted",
  outline = true,
}: Props) {
  const marker = useRef<HTMLButtonElement>(null)
  // Native listeners, because the menu listens natively and React's synthetic stopPropagation
  // runs after it. The menu handles arrows, typeahead, Enter and Space; keep them off this marker.
  useEffect(() => {
    const el = marker.current
    if (!el) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Tab" && e.key !== "Escape") e.stopPropagation()
    }
    el.addEventListener("keydown", onKeyDown)
    return () => el.removeEventListener("keydown", onKeyDown)
  }, [])
  return (
    <TipPopup content={content}>
      {(tip) => (
        <chakra.button
          type="button"
          aria-label={label}
          // Inside a menu, zag focuses the first tabbable on open; this marker swallows arrows, so skip it.
          data-no-autofocus=""
          ref={marker}
          {...tip}
          display="inline-flex"
          alignItems="center"
          justifyContent="center"
          flexShrink={0}
          borderWidth={variant === "chip" && !outline ? "0" : "1px"}
          lineHeight="1"
          cursor="pointer"
          {...(variant === "chip"
            ? {
                px: "7px",
                py: "1px",
                borderRadius: "full",
                borderColor: chipColor,
                color: chipColor,
                fontSize: "12px",
                style: { fontVariantNumeric: "tabular-nums" },
              }
            : markerStyle)}
        >
          {children}
        </chakra.button>
      )}
    </TipPopup>
  )
}
