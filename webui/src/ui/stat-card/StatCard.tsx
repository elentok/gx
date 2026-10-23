import { Box, Text } from "@chakra-ui/react";
import type { ComponentProps, ReactNode } from "react";
import { DayChart, type DayChartKind, type DaySeries } from "../day-chart/DayChart";
import { LineChart } from "../day-chart/LineChart";
import { Card } from "../card/Card";

interface StatCardProps {
  title: string;
  days: string[];
  series: DaySeries[];
  chart?: DayChartKind | "line";
  // Series folded into another one: listed in the hover tooltip, never drawn or counted.
  folded?: ComponentProps<typeof DayChart>["folded"];
  // Overrides for the special cards; the defaults are the sum and the daily average over days with data.
  headline?: ReactNode;
  small?: ReactNode;
}

export function StatCard({ title, days, series, chart = "bars", folded, headline, small }: StatCardProps) {
  const hasData = (i: number) => series.some((s) => s.values[i] !== null);
  const dataDays = days.filter((_, i) => hasData(i)).length;
  const partial = dataDays < days.length;
  const total = series.reduce((a, s) => a + s.values.reduce<number>((b, v) => b + (v ?? 0), 0), 0);
  const average = dataDays === 0 ? 0 : total / dataDays;

  return (
    <Card title={title}>
      <Box display="flex" alignItems="baseline" gap="2">
        <Text fontSize="2xl" fontWeight="semibold">
          {headline ?? total}
        </Text>
        {partial && (
          <Text as="span" fontSize="xs" color="text.dim">
            partial
          </Text>
        )}
      </Box>
      <Text fontSize="xs" color="text.muted" mb="2">
        {small ?? `${average.toFixed(1)}/day`}
      </Text>
      {chart === "line" ? <LineChart days={days} tone={series[0].tone} values={series[0].values} /> : <DayChart days={days} series={series} kind={chart} folded={folded} />}
      <Box as="ul" display="flex" gap="3" flexWrap="wrap" mt="2" fontSize="xs" listStyleType="none" aria-label="Legend">
        {series.map((s) => (
          <li key={s.key}>
            <Box as="span" display="inline-block" w="8px" h="8px" borderRadius="sm" mr="1" style={{ background: `var(--chakra-colors-tone-${s.tone}-fg)` }} />
            {s.label}
          </li>
        ))}
      </Box>
    </Card>
  );
}
