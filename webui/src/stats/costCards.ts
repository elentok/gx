import type { ComponentProps } from "react";
import type { StatsAmount, StatsCostGroup, StatsHarnessAmount } from "../generated/server";
import type { ChartColor, DaySeries } from "../ui/day-chart/DayChart";
import type { StatCard } from "../ui/stat-card/StatCard";
import type { Stats } from "./stats";

type CardProps = ComponentProps<typeof StatCard>;

const sum = (values: number[]) => values.reduce((a, v) => a + v, 0);
// A no-data day is null so the chart hatches it and the card counts it as partial.
export const amounts = (a: StatsAmount[]) => a.map((d) => (d.no_data ? null : d.value));
export const sumNullable = (values: (number | null)[]) => sum(values.map((v) => v ?? 0));

const GROUP_COLORS: Record<string, ChartColor> = { Queue: "blue", Design: "purple", "One-off": "green", Other: "muted" };
const compactFormat = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 0 });
export const compact = (n: number) => (n < 1000 ? String(Math.round(n)) : compactFormat.format(n));

export function costCardProps(
  stats: Stats,
  title: string,
  pick: (g: StatsCostGroup) => StatsAmount[],
  share: (h: StatsHarnessAmount) => number,
  fmt: (n: number) => string,
  note: (stampDays: boolean) => string,
): CardProps {
  const { days, cost: c } = stats;
  // The server marks only the groups that stamps cover (Queue, Design); the other groups read 0 that day. One no-data group makes the whole day no-data.
  const noData = days.map((_, i) => c.groups.some((g) => pick(g)[i]?.no_data));
  const series: DaySeries[] = c.groups.map((g) => ({
    key: g.group,
    label: g.group,
    tone: "muted",
    color: GROUP_COLORS[g.group] ?? "muted",
    values: amounts(pick(g)).map((v, i) => (noData[i] ? null : v)),
  }));
  const total = sum(series.map((s) => sumNullable(s.values)));
  const dataDays = days.filter((_, i) => series.some((s) => s.values[i] !== null)).length;
  // Rebuilt and stamp-only days are drawn lighter; a stamp day with no data hatches through the null values.
  const dayStyles = days.map((_, i) => (c.sources[i] === "live" || c.sources[i] === undefined ? undefined : ("dim" as const)));
  const dayNotes = days.map((_, i) => (c.sources[i] === "rebuilt" ? ["rebuilt from transcripts"] : c.sources[i] === "stamp" ? ["ticket stamps only"] : undefined));
  const extraRows = days.map((_, i) => {
    const split = (harness: string) => c.groups.reduce((a, g) => a + (g.harness[i]?.[harness] ? share(g.harness[i][harness]) : 0), 0);
    const claude = split("claude");
    const codex = split("codex");
    if (claude === 0 && codex === 0) return undefined;
    return [
      { label: "Claude", value: fmt(claude) },
      { label: "Codex", value: fmt(codex) },
    ];
  });
  return {
    title,
    days,
    chart: "stacked",
    series,
    headline: fmt(total),
    small: `${fmt(dataDays === 0 ? 0 : total / dataDays)}/day${note(c.sources.includes("stamp"))}`,
    partial: c.partial || dataDays < days.length,
    dayStyles,
    dayNotes,
    extraRows,
  };
}
