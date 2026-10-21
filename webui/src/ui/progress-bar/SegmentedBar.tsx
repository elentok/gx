import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";

export interface Segment {
  key: string;
  tone: Tone;
  count: number;
}

// Segments with a zero count are not drawn. `value` is what the progressbar role reports as done.
export function SegmentedBar({ segments, value, label }: { segments: Segment[]; value: number; label: string }) {
  const total = segments.reduce((sum, s) => sum + s.count, 0);
  return (
    <Box
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={total}
      aria-valuenow={value}
      h="bar"
      borderRadius="sm"
      overflow="hidden"
      display="flex"
      bg="tone.muted.bg"
    >
      {segments
        .filter((s) => s.count > 0)
        .map((s) => (
          <Box
            key={s.key}
            data-segment={s.key}
            data-tone={s.tone}
            h="100%"
            style={{ width: `${(s.count / total) * 100}%` }}
            bg={`tone.${s.tone}.fg`}
          />
        ))}
    </Box>
  );
}
