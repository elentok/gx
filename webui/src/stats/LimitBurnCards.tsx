import { Box, Text } from "@chakra-ui/react";
import {
  LimitForecastProjected,
  type StatsLimitBurn,
  type StatsLimitBurns,
} from "../generated/server";
import { forecastLine } from "../usage/format";
import { StatCard } from "../ui/stat-card/StatCard";

const monthDay = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
});

// Built from parts in local time: Date.parse of a "2026-10-09" key would be UTC and shift the day west of UTC.
function startLabel(dayKey: string): string {
  const [y, m, d] = dayKey.split("-").map(Number);
  return `starts ${monthDay.format(new Date(y, m - 1, d))}`;
}

const points = (n: number) => String(Math.round(n));

function LimitBurnCard({
  title,
  color,
  burn,
  days,
  weekly,
  project,
}: {
  title: string;
  color: "claude" | "codex";
  burn: StatsLimitBurn;
  days: string[];
  weekly: boolean;
  project: string;
}) {
  const values = burn.days.map((d) => (d.no_data ? null : d.burn));
  const showPace = weekly && burn.forecast?.status === LimitForecastProjected;
  const forecast = weekly ? forecastLine(burn.forecast) : undefined;
  const dayNotes = burn.days.map((d) =>
    d.reset_day ? ["↺ window reset"] : undefined,
  );
  const partial = burn.start_day !== undefined;

  return (
    <StatCard
      title={title}
      days={days}
      partial={partial}
      headline={`${points(burn.sum)} pts`}
      small={`${points(burn.average)} pts/day${partial ? ` · ${startLabel(burn.start_day!)}` : ""}`}
      series={[{ key: "burn", label: "Burn", tone: "muted", color, values }]}
      dayNotes={dayNotes}
      footer={
        <>
          {weekly ? (
            <Box
              display="flex"
              justifyContent="space-between"
              gap="3"
              mt="1"
              fontSize="xs"
              color="text.muted"
            >
              {showPace ? (
                <span>
                  7-day average: {points(burn.forecast!.pace)} points a day
                </span>
              ) : (
                <span />
              )}
              {forecast && (
                <span
                  style={
                    forecast.warn
                      ? { color: "var(--chakra-colors-tone-deadlocked-fg)" }
                      : undefined
                  }
                >
                  {forecast.text}
                </span>
              )}
            </Box>
          ) : (
            <Text fontSize="xs" color="text.muted" mt="1">
              Reset days are marked ↺ in the tooltip.
            </Text>
          )}
          {project && (
            <>
              <Text
                as="span"
                fontSize="xs"
                color="text.dim"
                display="block"
                mt="1"
              >
                All projects
              </Text>
              <Text fontSize="xs" color="text.dim">
                The limit counts all use on your account
              </Text>
            </>
          )}
        </>
      }
    />
  );
}

export function LimitBurnCards({
  days,
  burns,
  project,
}: {
  days: string[];
  burns: StatsLimitBurns;
  project: string;
}) {
  const card = (
    title: string,
    color: "claude" | "codex",
    burn: StatsLimitBurn,
    weekly: boolean,
  ) => (
    <LimitBurnCard
      title={title}
      color={color}
      burn={burn}
      days={days}
      weekly={weekly}
      project={project}
    />
  );
  return (
    <>
      {card("Claude weekly burn", "claude", burns.claude_weekly, true)}
      {card("Codex weekly burn", "codex", burns.codex_weekly, true)}
      {card("Claude 5-hour burn", "claude", burns.claude_5h, false)}
      {card("Codex 5-hour burn", "codex", burns.codex_5h, false)}
    </>
  );
}
