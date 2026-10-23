import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";

// Small spinning arc that marks a live ticket or epic inline, next to its name.
export function Arc({ tone = "running", label }: { tone?: Tone; label: string }) {
  return (
    <Box
      as="span"
      role="img"
      aria-label={label}
      data-tone={tone}
      display="inline-block"
      flexShrink={0}
      w="3"
      h="3"
      verticalAlign="middle"
      borderRadius="full"
      borderWidth="2px"
      borderColor="transparent"
      borderTopColor={`tone.${tone}.fg`}
      borderRightColor={`tone.${tone}.fg`}
      animation="spin 1.05s linear infinite"
      _motionReduce={{ animation: "none" }}
    />
  );
}
