import { Box, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import type { Tone } from "../tone"

interface CardProps {
  title?: string
  tone?: Tone
  // Runs a light round the border in the tone's color, for an epic with work in flight.
  live?: boolean
  children: ReactNode
  /** Caps the card's height. Its boxes become shrinkable columns, so a child with `minH="0"` can scroll. */
  maxHeight?: string
  /** Names the card's outer box for the mock check. */
  "data-mock-part"?: string
}

export function Card({
  title,
  tone = "muted",
  live,
  children,
  maxHeight,
  "data-mock-part": mockPart,
}: CardProps) {
  const column =
    maxHeight === undefined
      ? undefined
      : ({ display: "flex", flexDirection: "column", minH: "0" } as const)
  const capped = column && { ...column, maxH: maxHeight }
  const body = (
    <>
      {title && (
        <Text fontFamily="condensed" fontSize="lg" fontWeight="600" mb="2">
          {title}
        </Text>
      )}
      {children}
    </>
  )
  if (!live) {
    return (
      <Box
        data-tone={tone}
        data-mock-part={mockPart}
        bg="surface.raised"
        borderWidth="1px"
        borderColor="border"
        borderRadius="md"
        p="3"
        {...capped}
      >
        {body}
      </Box>
    )
  }
  // The 1px padding shows the rotating gradient behind the body as the border.
  return (
    <Box
      data-tone={tone}
      data-live
      data-mock-part={mockPart}
      borderRadius="md"
      p="1px"
      animation="orbit 6s linear infinite"
      _motionReduce={{ animation: "none" }}
      style={{
        background: `conic-gradient(from var(--gx-orbit-angle), var(--chakra-colors-border) 0deg 250deg, var(--chakra-colors-tone-${tone}-fg) 320deg, var(--chakra-colors-border) 360deg)`,
      }}
      {...capped}
    >
      <Box bg="surface.raised" borderRadius="calc(var(--chakra-radii-md) - 1px)" p="3" {...column}>
        {body}
      </Box>
    </Box>
  )
}
