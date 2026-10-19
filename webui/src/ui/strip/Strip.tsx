import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

// Thin full-width banner, e.g. the Offline notice.
export function Strip({ tone = "muted", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <Box
      role="status"
      data-tone={tone}
      w="100%"
      px="4"
      py="1"
      textAlign="center"
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
