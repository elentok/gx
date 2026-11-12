import { chakra, type HTMLChakraProps } from "@chakra-ui/react"
import { forwardRef, type ElementType, type ReactNode } from "react"
import { useDevice } from "../useWide"

// Touch layouts (330px and 344px sidebars) fit 34px. At the 280px desktop sidebar, the + New button, both
// trays and the gaps leave room for four boxes of this size and no more; shrinking is off, so it is exact.
export const DESKTOP_BOX = "25px"
export const TOUCH_BOX = "34px"

export const useFooterBox = () => (useDevice() === "desktop" ? DESKTOP_BOX : TOUCH_BOX)

export type BadgeTone = "waiting" | "running"

/** The one badge both counters use, so size, font and corner agree by construction. */
export function FooterBadge({ tone, children }: { tone: BadgeTone; children: ReactNode }) {
  return (
    <chakra.span
      aria-hidden
      data-part="badge"
      position="absolute"
      top="-3px"
      right="-3px"
      minW="18px"
      h="18px"
      px="5px"
      textAlign="center"
      lineHeight="18px"
      borderRadius="full"
      bg={`tone.${tone}.fg`}
      color="surface"
      fontSize="11px"
      fontWeight="700"
    >
      {children}
    </chakra.span>
  )
}

type Props = Omit<HTMLChakraProps<"button">, "children"> & {
  as?: ElementType
  href?: string
  badge?: { tone: BadgeTone; text: ReactNode }
  children: ReactNode
}

/** A fixed-size footer box that never shrinks. It renders the link, button and indicator variants through `as`. */
export const FooterButton = forwardRef<HTMLElement, Props>(function FooterButton(
  { badge, children, ...rest },
  ref,
) {
  const size = useFooterBox()
  return (
    <chakra.button
      ref={ref}
      type={rest.as ? undefined : "button"}
      position="relative"
      flex="none"
      boxSize={size}
      minW={size}
      minH={size}
      p="0"
      borderRadius="8px"
      display="grid"
      placeItems="center"
      {...rest}
    >
      {children}
      {badge ? <FooterBadge tone={badge.tone}>{badge.text}</FooterBadge> : null}
    </chakra.button>
  )
})

/** The one hover/open look every footer control shares (a tint, no border change). */
export const footerHover = {
  _hover: { bg: "tone.muted.bg" },
  _open: { bg: "tone.muted.bg" },
} as const

/** The clickable variants' look; indicators omit it. */
export const interactiveStyle = {
  color: "text.dim",
  cursor: "pointer",
  ...footerHover,
} as const
