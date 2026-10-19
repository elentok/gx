import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";

export function LiveDot({ tone = "running", label }: { tone?: Tone; label: string }) {
  return (
    <Box
      as="span"
      role="status"
      aria-label={label}
      data-tone={tone}
      display="inline-block"
      w="2"
      h="2"
      borderRadius="full"
      bg={`tone.${tone}.fg`}
    />
  );
}
