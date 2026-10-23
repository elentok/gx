import type { Tone } from "../tone";

const SLOT = 10;
const toneVar = (tone: Tone) => `var(--chakra-colors-tone-${tone}-fg)`;

interface LineChartProps {
  days: string[];
  tone: Tone;
  // One value per day; null means no data for that day and leaves a gap in the line.
  values: (number | null)[];
  height?: number;
}

export function LineChart({ days, tone, values, height = 110 }: LineChartProps) {
  const max = Math.max(1, ...values.map((v) => v ?? 0));
  const point = (v: number, i: number) => [i * SLOT + SLOT / 2, height - (v / max) * (height - 4) - 2] as const;

  // A null day ends the current run, so the line never bridges a gap.
  const runs: (readonly [number, number])[][] = [[]];
  values.forEach((v, i) => {
    if (v === null) runs.push([]);
    else runs[runs.length - 1].push(point(v, i));
  });

  return (
    <svg viewBox={`0 0 ${days.length * SLOT} ${height}`} preserveAspectRatio="none" width="100%" height={height} style={{ display: "block" }}>
      <line x1={0} x2={days.length * SLOT} y1={height - 1} y2={height - 1} stroke="var(--chakra-colors-border)" />
      {runs
        .filter((r) => r.length > 1)
        .map((r, i) => (
          <polyline key={i} points={r.map((p) => p.join(",")).join(" ")} fill="none" stroke={toneVar(tone)} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
        ))}
      {values.map((v, i) => {
        if (v === null) return null;
        const [cx, cy] = point(v, i);
        return <circle key={days[i]} data-point={i} cx={cx} cy={cy} r="2" fill={toneVar(tone)} />;
      })}
    </svg>
  );
}
