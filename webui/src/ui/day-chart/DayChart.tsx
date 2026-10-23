import { Box, Text } from "@chakra-ui/react";
import { useId, useState, type ReactNode } from "react";
import type { Tone } from "../tone";

export interface DaySeries {
  key: string;
  label: string;
  tone: Tone;
  // One value per day; null means no data for that day.
  values: (number | null)[];
}

// "stacked" and "side-by-side" share one scale; "diverging" draws the second series below the baseline.
export type DayChartKind = "bars" | "stacked" | "side-by-side" | "diverging";

interface DayChartProps {
  days: string[];
  series: DaySeries[];
  kind?: DayChartKind;
  // Series the chart folded into "other": listed in the tooltip, never drawn.
  folded?: Omit<DaySeries, "tone">[];
  height?: number;
}

const SLOT = 10;
const BAR = 8;
const toneVar = (tone: Tone) => `var(--chakra-colors-tone-${tone}-fg)`;

export function DayChart({ days, series, kind = "bars", folded = [], height = 110 }: DayChartProps) {
  const patternId = useId();
  const [hover, setHover] = useState<number | null>(null);
  const width = days.length * SLOT;
  const noData = (i: number) => series.every((s) => s.values[i] === null);
  const val = (s: DaySeries, i: number) => s.values[i] ?? 0;

  let up = 1;
  let down = 0;
  days.forEach((_, i) => {
    if (kind === "stacked") up = Math.max(up, series.reduce((a, s) => a + val(s, i), 0));
    else if (kind === "diverging") {
      up = Math.max(up, val(series[0], i));
      down = Math.max(down, val(series[1], i));
    } else up = Math.max(up, ...series.map((s) => val(s, i)));
  });
  const baseY = (height * up) / (up + down);
  const scale = baseY / up;

  const marks: ReactNode[] = [];
  days.forEach((_, i) => {
    const x = i * SLOT;
    if (noData(i)) {
      marks.push(<rect key={`nd${i}`} data-nodata={i} x={x} y={0} width={BAR} height={height} fill={`url(#${patternId})`} />);
      return;
    }
    const bar = (s: DaySeries, bx: number, y: number, w: number, h: number, below = false) =>
      h > 0 && (
        <rect key={`${i}-${s.key}`} data-bar={i} data-series={s.key} data-below={below || undefined} x={bx} y={y} width={w} height={h} fill={toneVar(s.tone)} />
      );
    if (kind === "side-by-side") {
      const w = BAR / series.length;
      series.forEach((s, si) => marks.push(bar(s, x + si * w, baseY - val(s, i) * scale, w, val(s, i) * scale)));
    } else if (kind === "diverging") {
      const [add, rem] = series;
      marks.push(bar(add, x, baseY - val(add, i) * scale, BAR, val(add, i) * scale));
      marks.push(bar(rem, x, baseY, BAR, val(rem, i) * scale, true));
    } else {
      let y = baseY;
      series.forEach((s) => {
        const h = val(s, i) * scale;
        y -= h;
        marks.push(bar(s, x, y, BAR, h));
      });
    }
  });

  const rows = hover === null ? [] : [...series, ...folded.map((f) => ({ ...f, tone: "muted" as Tone }))];
  const fmtValue = (v: number | null | undefined) => (v === null || v === undefined ? "no data" : String(v));

  return (
    <Box position="relative" w="full" minW="0" onMouseLeave={() => setHover(null)}>
      <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" width="100%" height={height} style={{ display: "block" }}>
        <defs>
          <pattern id={patternId} width="3" height="3" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
            <line x1="0" y1="0" x2="0" y2="3" stroke={toneVar("muted")} strokeWidth="1" />
          </pattern>
        </defs>
        <line data-baseline x1={0} x2={width} y1={baseY} y2={baseY} stroke="var(--chakra-colors-border)" />
        {marks}
        {days.map((d, i) => (
          <rect
            key={d}
            data-hit={i}
            x={i * SLOT}
            y={0}
            width={SLOT}
            height={height}
            fill="transparent"
            onMouseEnter={() => setHover(i)}
            onClick={() => setHover(hover === i ? null : i)}
          />
        ))}
      </svg>
      {hover !== null && (
        <Box
          role="tooltip"
          position="absolute"
          top="0"
          left={`${((hover + 0.5) / days.length) * 100}%`}
          transform={hover > days.length / 2 ? "translateX(-100%)" : undefined}
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          borderRadius="sm"
          px="2"
          py="1"
          fontSize="xs"
          pointerEvents="none"
          zIndex={1}
        >
          <Text color="text.muted">{days[hover]}</Text>
          {noData(hover) ? (
            <Text color="text.dim">no data</Text>
          ) : (
            rows.map((s) => (
              <Text key={s.key} whiteSpace="nowrap">
                <Box as="span" display="inline-block" w="8px" h="8px" borderRadius="sm" mr="1" style={{ background: toneVar(s.tone) }} />
                {s.label}: {fmtValue(s.values[hover])}
              </Text>
            ))
          )}
        </Box>
      )}
    </Box>
  );
}
