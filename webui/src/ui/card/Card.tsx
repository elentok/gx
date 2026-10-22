import { Box, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

interface CardProps {
  title?: string;
  tone?: Tone;
  // Runs a light round the border in the tone's color, for an epic with work in flight.
  live?: boolean;
  children: ReactNode;
}

export function Card({ title, tone = "muted", live, children }: CardProps) {
  const body = (
    <>
      {title && (
        <Text fontFamily="condensed" fontSize="lg" fontWeight="600" mb="2">
          {title}
        </Text>
      )}
      {children}
    </>
  );
  if (!live) {
    return (
      <Box data-tone={tone} bg="surface.raised" borderWidth="1px" borderColor="border" borderRadius="md" p="3">
        {body}
      </Box>
    );
  }
  // The 1px padding shows the rotating gradient behind the body as the border.
  return (
    <Box
      data-tone={tone}
      data-live
      borderRadius="md"
      p="1px"
      animation="orbit 6s linear infinite"
      _motionReduce={{ animation: "none" }}
      style={{
        background: `conic-gradient(from var(--gx-orbit-angle), var(--chakra-colors-border) 0deg 250deg, var(--chakra-colors-tone-${tone}-fg) 320deg, var(--chakra-colors-border) 360deg)`,
      }}
    >
      <Box bg="surface.raised" borderRadius="calc(var(--chakra-radii-md) - 1px)" p="3">
        {body}
      </Box>
    </Box>
  );
}
