import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";

export function ProgressBar({ value, max, tone = "running" }: { value: number; max: number; tone?: Tone }) {
  const pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  return (
    <Box
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      data-tone={tone}
      h="bar"
      borderRadius="sm"
      overflow="hidden"
      bg={`tone.${tone}.bg`}
    >
      <Box data-fill h="100%" style={{ width: `${pct}%` }} bg={`tone.${tone}.fg`} />
    </Box>
  );
}
