import { Box, Spinner as ChakraSpinner } from "@chakra-ui/react";
import type { Tone } from "../tone";

// Centered in both directions in the space the page leaves, for a page that has no data yet.
export function Spinner({ label, tone = "muted" }: { label: string; tone?: Tone }) {
  return (
    <Box
      role="status"
      aria-label={label}
      data-tone={tone}
      display="flex"
      alignItems="center"
      justifyContent="center"
      minH="70vh"
    >
      <ChakraSpinner size="xl" borderWidth="3px" color={`tone.${tone}.fg`} />
    </Box>
  );
}
