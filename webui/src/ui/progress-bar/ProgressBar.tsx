import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";

interface Props {
  value: number;
  max: number;
  tone?: Tone;
  /** A 4px bar on the neutral hairline track, as the menu's limits rows draw it. */
  thin?: boolean;
}

export function ProgressBar({ value, max, tone = "running", thin = false }: Props) {
  const pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  return (
    <Box
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      data-tone={tone}
      data-thin={thin || undefined}
      h={thin ? "barThin" : "bar"}
      borderRadius={thin ? "2px" : "sm"}
      overflow="hidden"
      bg={thin ? "hairline" : `tone.${tone}.bg`}
    >
      <Box data-fill h="100%" style={{ width: `${pct}%` }} bg={`tone.${tone}.fg`} />
    </Box>
  );
}
