import { Box, chakra, Text } from "@chakra-ui/react";
import { useSearchParams } from "react-router";
import { useConnectionContext } from "../connection";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { DetailPage } from "../page/DetailPage";
import { SectionHeading } from "../page/SectionHeading";
import { Button } from "../ui/button/Button";
import type { ReactNode } from "react";
import type { StatsAmount } from "../generated/server";
import type { DaySeries } from "../ui/day-chart/DayChart";
import { Spinner } from "../ui/spinner/Spinner";
import { StatCard } from "../ui/stat-card/StatCard";
import type { Tone } from "../ui/tone";
import { parseRange, RANGES, useStats, type FetchStats, type Stats } from "./stats";

const sum = (values: number[]) => values.reduce((a, v) => a + v, 0);
const perDay = (values: number[]) => (values.length === 0 ? 0 : sum(values) / values.length).toFixed(1);

function Section({ id, title, tone, children }: { id: string; title: string; tone: Tone; children: ReactNode }) {
  return (
    <Box mt="6" role="region" aria-labelledby={id}>
      <SectionHeading id={id} tone={tone}>
        {title}
      </SectionHeading>
      <Box display="grid" gridTemplateColumns={{ base: "1fr", md: "1fr 1fr" }} gap="4">
        {children}
      </Box>
    </Box>
  );
}

function Throughput({ stats }: { stats: Stats }) {
  const { days, throughput: t } = stats;
  return (
    <Section id="stats-throughput" title="Throughput" tone="running">
        <StatCard title="Tickets merged" days={days} series={[{ key: "tickets", label: "Tickets", tone: "running", values: t.tickets_merged }]} />
        <StatCard title="Epics merged to trunk" days={days} series={[{ key: "epics", label: "Epics", tone: "done", values: t.epics_merged }]} />
        <StatCard
          title="Lines of code"
          days={days}
          chart="diverging"
          headline={`+${sum(t.lines_added)} −${sum(t.lines_removed)}`}
          small=""
          series={[
            { key: "added", label: "Added", tone: "running", values: t.lines_added },
            { key: "removed", label: "Removed", tone: "deadlocked", values: t.lines_removed },
          ]}
        />
        <StatCard
          title="Follow-ups"
          days={days}
          chart="side-by-side"
          headline={`${sum(t.follow_ups_created)} created · ${sum(t.follow_ups_finished)} finished`}
          small={`${perDay(t.follow_ups_created)}/day created · ${perDay(t.follow_ups_finished)}/day finished`}
          series={[
            { key: "created", label: "Created", tone: "waiting", values: t.follow_ups_created },
            { key: "finished", label: "Finished", tone: "done", values: t.follow_ups_finished },
          ]}
        />
        <StatCard
          title="Waiting for design"
          days={days}
          chart="line"
          headline={t.waiting_for_design.at(-1) ?? 0}
          small="maps open now"
          series={[{ key: "waiting", label: "Open maps", tone: "waiting", values: t.waiting_for_design }]}
        />
    </Section>
  );
}

// A no-data day is null so the chart hatches it and the card counts it as partial.
const amounts = (a: StatsAmount[]) => a.map((d) => (d.no_data ? null : d.value));
const sumNullable = (values: (number | null)[]) => sum(values.map((v) => v ?? 0));

function Cost({ stats }: { stats: Stats }) {
  const { days, cost: c } = stats;
  const card = (title: string, design: StatsAmount[], impl: StatsAmount[], fmt: (n: number) => string) => {
    const series: DaySeries[] = [
      { key: "design", label: "Design", tone: "waiting", values: amounts(design) },
      { key: "implementation", label: "Implementation", tone: "running", values: amounts(impl) },
    ];
    const total = sum(series.map((s) => sumNullable(s.values)));
    const dataDays = days.filter((_, i) => series.some((s) => s.values[i] !== null)).length;
    return (
      <StatCard title={title} days={days} chart="stacked" series={series} headline={fmt(total)} small={`${fmt(dataDays === 0 ? 0 : total / dataDays)}/day`} />
    );
  };
  return (
    <Section id="stats-cost" title="Cost and effort" tone="waiting">
      {card("Tokens", c.tokens_design, c.tokens_implementation, thousands)}
      {card("Spend", c.spend_design, c.spend_implementation, (n) => `$${n.toFixed(2)}`)}
    </Section>
  );
}

const thousands = (n: number) => Math.round(n).toLocaleString("en-US");

function Grilling({ stats }: { stats: Stats }) {
  const { days, grilling: g } = stats;
  const card = (title: string, label: string, data: StatsAmount[]) => {
    const values = amounts(data);
    const total = sumNullable(values);
    const dataDays = values.filter((v) => v !== null).length;
    return (
      <StatCard
        title={title}
        days={days}
        series={[{ key: "count", label, tone: "done", values }]}
        headline={thousands(total)}
        small={`${thousands(dataDays === 0 ? 0 : total / dataDays)}/day`}
      />
    );
  };
  return (
    <Section id="stats-grilling" title="Grilling" tone="done">
      {card("Words read", "Words", g.words)}
      {card("Questions asked", "Questions", g.questions)}
    </Section>
  );
}

const REASON_TONES: Tone[] = ["paused", "waiting", "running", "deadlocked", "done"];
const ENDING_SOURCES = ["web", "tui", "cli", "gx", "unknown"];
const ENDING_LABELS: Record<string, string> = { web: "Web", tui: "TUI", cli: "CLI", gx: "gx", unknown: "Unknown" };

const countSeries = (key: string, days: { [key: string]: number }[]) => days.map((d) => d[key] ?? 0);
const keysOf = (days: { [key: string]: number }[]) => [...new Set(days.flatMap((d) => Object.keys(d)))].sort();

function formatDuration(secs: number) {
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.round(secs / 60)}m`;
  return `${Math.floor(secs / 3600)}h ${Math.round((secs % 3600) / 60)}m`;
}

function Friction({ stats }: { stats: Stats }) {
  const { days, friction: f } = stats;

  // The server names the top reasons (plus "other" when more exist); the rest are summed into "other" and listed in the tooltip.
  const top = f.pause_reasons.filter((r) => r !== "other");
  const foldedKeys = keysOf(f.pauses_by_reason).filter((k) => !top.includes(k));
  const pauseSeries: DaySeries[] = top.map((r, i) => ({ key: r, label: r, tone: REASON_TONES[i % REASON_TONES.length], values: countSeries(r, f.pauses_by_reason) }));
  if (f.pause_reasons.includes("other")) {
    pauseSeries.push({ key: "other", label: "other", tone: "muted", values: f.pauses_by_reason.map((d) => sum(foldedKeys.map((k) => d[k] ?? 0))) });
  }
  const folded = foldedKeys.map((k) => ({ key: k, label: k, values: countSeries(k, f.pauses_by_reason) }));

  const eventSeries: DaySeries[] = keysOf(f.run_events).map((k, i) => ({ key: k, label: k, tone: REASON_TONES[i % REASON_TONES.length], values: countSeries(k, f.run_events) }));
  const endingSeries: DaySeries[] = ENDING_SOURCES.map((k, i) => ({ key: k, label: ENDING_LABELS[k], tone: REASON_TONES[i % REASON_TONES.length], values: countSeries(k, f.pause_endings) }));

  return (
    <Section id="stats-friction" title="Friction" tone="paused">
      <StatCard title="Pauses" days={days} chart="stacked" series={pauseSeries} folded={folded} />
      <StatCard title="Run events" days={days} chart="stacked" series={eventSeries} />
      <StatCard
        title="Pause endings"
        days={days}
        chart="stacked"
        series={endingSeries}
        headline={f.web_share === undefined ? "—" : `${Math.round(f.web_share * 100)}% web`}
        small={f.median_resume_secs === undefined ? "" : `median ${formatDuration(f.median_resume_secs)} to resume`}
      />
    </Section>
  );
}

const selectProps = { fontSize: "sm", px: "2", py: "1", borderWidth: "1px", borderRadius: "md", bg: "transparent" } as const;

export function StatsPage({ fetchStats, fetchEpics }: { fetchStats?: FetchStats; fetchEpics?: FetchEpicList }) {
  const conn = useConnectionContext();
  const [params, setParams] = useSearchParams();
  const range = parseRange(params.get("range"));
  const project = params.get("project") ?? "";
  const { stats, failed, retry } = useStats({ range, project }, fetchStats, conn);
  const { list } = useEpicList(fetchEpics, conn);

  const projects = [...new Set([...(list?.epics.map((e) => e.project) ?? []), ...(project ? [project] : [])])].sort();
  const set = (key: string, value: string) =>
    setParams((p) => {
      const next = new URLSearchParams(p);
      if (value) next.set(key, value);
      else next.delete(key);
      return next;
    });

  return (
    <DetailPage active="stats" mode="wide">
      <Box display="flex" alignItems="center" gap="4" mt="4">
        <chakra.select aria-label="Project" value={project} onChange={(e) => set("project", e.target.value)} {...selectProps}>
          <option value="">All projects</option>
          {projects.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </chakra.select>
        <chakra.select aria-label="Range" value={range} onChange={(e) => set("range", e.target.value)} {...selectProps}>
          {RANGES.map((r) => (
            <option key={r} value={r}>
              {r}d
            </option>
          ))}
        </chakra.select>
      </Box>
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load stats</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !stats ? (
        <Spinner label="Loading stats" />
      ) : (
        <>
          <Throughput stats={stats} />
          <Cost stats={stats} />
          <Grilling stats={stats} />
          <Friction stats={stats} />
        </>
      )}
    </DetailPage>
  );
}
