import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

// Pill for queue-level states such as Paused.
export function Chip({ tone = "muted", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <Box
      as="span"
      data-tone={tone}
      display="inline-block"
      whiteSpace="nowrap"
      px="3"
      borderRadius="lg"
      borderWidth="1px"
      borderColor={`tone.${tone}.fg`}
      fontFamily="condensed"
      fontSize="sm"
      fontWeight="600"
      color={`tone.${tone}.fg`}
      bg={`tone.${tone}.bg`}
    >
      {children}
    </Box>
  );
}
