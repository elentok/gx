import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

export function Badge({ tone = "muted", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <Box
      as="span"
      data-tone={tone}
      display="inline-block"
      px="2"
      borderRadius="sm"
      fontFamily="condensed"
      fontSize="xs"
      fontWeight="600"
      textTransform="uppercase"
      color={`tone.${tone}.fg`}
      bg={`tone.${tone}.bg`}
    >
      {children}
    </Box>
  );
}
