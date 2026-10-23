import { Box, chakra, Text } from "@chakra-ui/react";
import { useSearchParams } from "react-router";
import { useConnectionContext } from "../connection";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { DetailPage } from "../page/DetailPage";
import { SectionHeading } from "../page/SectionHeading";
import { Button } from "../ui/button/Button";
import { Spinner } from "../ui/spinner/Spinner";
import { StatCard } from "../ui/stat-card/StatCard";
import { parseRange, RANGES, useStats, type FetchStats, type Stats } from "./stats";

const sum = (values: number[]) => values.reduce((a, v) => a + v, 0);
const perDay = (values: number[]) => (values.length === 0 ? 0 : sum(values) / values.length).toFixed(1);

function Throughput({ stats }: { stats: Stats }) {
  const { days, throughput: t } = stats;
  return (
    <Box mt="6" role="region" aria-labelledby="stats-throughput">
      <SectionHeading id="stats-throughput" tone="running">
        Throughput
      </SectionHeading>
      <Box display="grid" gridTemplateColumns={{ base: "1fr", md: "1fr 1fr" }} gap="4">
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
          small=""
          series={[{ key: "waiting", label: "Epics", tone: "waiting", values: t.waiting_for_design }]}
        />
      </Box>
    </Box>
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
    <DetailPage active="stats">
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
          <Button variant="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !stats ? (
        <Spinner label="Loading stats" />
      ) : (
        <Throughput stats={stats} />
      )}
    </DetailPage>
  );
}
