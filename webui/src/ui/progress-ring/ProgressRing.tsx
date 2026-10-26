import { Box, chakra } from "@chakra-ui/react";
import type { Kind, Tone } from "../tone";

export const KIND_LABELS: Record<Kind, string> = {
  design: "Design",
  implementation: "Implementation",
  oneoff: "One-off",
  followups: "Follow-ups",
};

const KIND_ICONS: Record<Kind, string> = {
  design: "M3 13l.7-3L10.5 3.2a1.4 1.4 0 0 1 2 2L5.7 12 3 13zM9.2 4.5l2.3 2.3",
  implementation: "M5.5 5L2.5 8l3 3M10.5 5l3 3-3 3M9 3.5l-2 9",
  oneoff: "M9 1.5L3.5 9H8l-1 5.5L12.5 7H8l1-5.5z",
  followups: "M2 9l2-5.5h8L14 9v3.5H2V9zM2 9h3.5l.8 1.5h3.4L10.5 9H14",
};

const SIZES = {
  md: { px: 64, stroke: 5, font: "15px", badge: true },
  sm: { px: 52, stroke: 4, font: "13px", badge: true },
  compact: { px: 26, stroke: 3, font: "9px", badge: false },
} as const;

// Both drawn on a 100-unit path so the comet's 16% share does not depend on the radius.
const COMET_SEGMENTS = 10;
const COMET_SHARE = 16;
const SEGMENT = COMET_SHARE / COMET_SEGMENTS;

interface Props {
  // 0..1
  progress: number;
  tone?: Tone;
  kind: Kind;
  // A percentage, n/m, "…" or empty.
  label?: string;
  size?: keyof typeof SIZES;
  // A white comet circles the ring while the work is live.
  live?: boolean;
}

export function ProgressRing({ progress, tone = "running", kind, label = "", size = "md", live = false }: Props) {
  const { px, stroke, font, badge } = SIZES[size];
  const r = (px - stroke) / 2;
  const c = px / 2;
  const clamped = Math.min(1, Math.max(0, progress));
  return (
    <Box position="relative" flexShrink={0} w={`${px}px`} h={`${px}px`} data-kind={kind} data-tone={tone}>
      <chakra.svg viewBox={`0 0 ${px} ${px}`} w="100%" h="100%" fill="none" strokeWidth={stroke} aria-hidden>
        <g transform={`rotate(-90 ${c} ${c})`}>
          <chakra.circle cx={c} cy={c} r={r} stroke="border" />
          <chakra.circle
            cx={c}
            cy={c}
            r={r}
            stroke={`tone.${tone}.fg`}
            strokeLinecap="round"
            pathLength={100}
            strokeDasharray={`${clamped * 100} 100`}
            display={clamped > 0 ? undefined : "none"}
          />
        </g>
      </chakra.svg>
      {live && (
        <chakra.svg
          data-testid="comet"
          viewBox={`0 0 ${px} ${px}`}
          position="absolute"
          inset="0"
          w="100%"
          h="100%"
          fill="none"
          strokeWidth={stroke}
          stroke="white"
          aria-hidden
          animation="spin 1.6s linear infinite"
          _motionReduce={{ animation: "none" }}
        >
          <g transform={`rotate(-90 ${c} ${c})`}>
            {Array.from({ length: COMET_SEGMENTS }, (_, i) => (
              <circle
                key={i}
                cx={c}
                cy={c}
                r={r}
                pathLength={100}
                strokeDasharray={`${SEGMENT} ${100 - SEGMENT}`}
                strokeDashoffset={-i * SEGMENT}
                opacity={(i + 1) / COMET_SEGMENTS}
              />
            ))}
          </g>
        </chakra.svg>
      )}
      {label && (
        <Box
          position="absolute"
          inset="0"
          display="flex"
          alignItems="center"
          justifyContent="center"
          fontFamily="condensed"
          fontSize={font}
          fontVariantNumeric="tabular-nums"
          lineHeight="1"
        >
          {label}
        </Box>
      )}
      {badge && (
        <Box
          role="img"
          aria-label={KIND_LABELS[kind]}
          position="absolute"
          right="-4px"
          bottom="-4px"
          w="26px"
          h="26px"
          borderRadius="full"
          borderWidth="2px"
          borderColor="surface.raised"
          bg="surface.raised"
          color={`kind.${kind}.fg`}
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          <chakra.svg
            viewBox="0 0 16 16"
            w="14px"
            h="14px"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d={KIND_ICONS[kind]} />
          </chakra.svg>
        </Box>
      )}
    </Box>
  );
}
