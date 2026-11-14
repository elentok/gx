import { Box, chakra, Text } from "@chakra-ui/react"
import type { StatsWebPerfCard, StatsWebPerformance } from "../generated/server"
import { SectionHeading } from "../page/SectionHeading"
import { Button } from "../ui/button/Button"
import { Chip } from "../ui/chip/Chip"
import type { Tone } from "../ui/tone"

export type PerfTab = "hard" | "route"

const MIN_SAMPLES = 20

const LABELS: Record<string, string> = {
  lcp: "LCP",
  inp: "INP",
  cls: "CLS",
  fcp: "FCP",
  app_ready: "App ready",
  route_ready: "Route ready",
}

const DEFINITIONS: Record<string, string> = {
  lcp: "Largest Contentful Paint: when the main content appears.",
  inp: "Interaction to Next Paint: how fast the page reacts to clicks and keys.",
  cls: "Cumulative Layout Shift: how much the layout jumps while loading.",
  fcp: "First Contentful Paint: when the first content appears. May be only the loading screen.",
  app_ready: "Time until the first usable gx screen.",
  route_ready: "Time from a navigation until the new route's content and data are ready.",
}

const STATUS: Record<string, { label: string; tone: Tone }> = {
  good: { label: "good", tone: "done" },
  "needs-improvement": { label: "needs improvement", tone: "waiting" },
  poor: { label: "poor", tone: "deadlocked" },
}

export function formatPerfValue(metric: string, v: number): string {
  if (metric === "cls") return v.toFixed(2)
  return v >= 1000 ? `${(v / 1000).toFixed(2)} s` : `${Math.round(v)} ms`
}

function Change({ pct, range }: { pct: number | undefined; range: number }) {
  if (pct === undefined) return <Text color="text.muted">—</Text>
  const rounded = Math.round(pct)
  const tone = rounded < 0 ? "tone.done.fg" : rounded > 0 ? "tone.deadlocked.fg" : "text.muted"
  return (
    <Text color={tone}>
      {rounded > 0 ? "+" : ""}
      {rounded}% vs previous {range}d
    </Text>
  )
}

function MetricCard({ card, range }: { card: StatsWebPerfCard; range: number }) {
  const low = card.count < MIN_SAMPLES
  const status = !low && card.status ? STATUS[card.status] : undefined
  const noThreshold = !low && !status && !(card.metric in { lcp: 1, inp: 1, cls: 1, fcp: 1 })
  return (
    <Box
      data-testid={`perf-card-${card.metric}`}
      data-low={low || undefined}
      borderWidth="1px"
      borderRadius="md"
      p="3"
      display="flex"
      flexDirection="column"
      gap="1"
    >
      <Text color="text.muted">{LABELS[card.metric] ?? card.metric}</Text>
      <Text fontSize="2xl" fontWeight="600" opacity={low ? 0.55 : 1}>
        {card.p75 === undefined ? "—" : formatPerfValue(card.metric, card.p75)}
      </Text>
      <Text color="text.muted">{card.count} samples</Text>
      {low ? (
        <Box>
          <Chip tone="muted">not enough samples</Chip>
        </Box>
      ) : status ? (
        <Box>
          <Chip tone={status.tone}>{status.label}</Chip>
        </Box>
      ) : noThreshold ? (
        <Text fontSize="xs" color="text.muted">
          no standard threshold
        </Text>
      ) : null}
      <Change pct={card.change_pct} range={range} />
      <Text mt="1" pt="1" borderTopWidth="1px" fontSize="xs" color="text.muted" opacity={0.8}>
        {DEFINITIONS[card.metric] ?? ""}
      </Text>
    </Box>
  )
}

const TABS: { id: PerfTab; label: string }[] = [
  { id: "hard", label: "Hard loads" },
  { id: "route", label: "Route changes" },
]

export function WebPerformance({
  data,
  range,
  tab,
  onTab,
  includeDev,
  onIncludeDev,
}: {
  data: StatsWebPerformance
  range: number
  tab: PerfTab
  onTab: (tab: PerfTab) => void
  includeDev: boolean
  onIncludeDev: (on: boolean) => void
}) {
  const cards = tab === "route" ? data.route_changes : data.hard_loads
  const empty = [...data.hard_loads, ...data.route_changes].every((c) => c.count === 0)
  return (
    <Box mt="6" role="region" aria-labelledby="stats-web-performance">
      <SectionHeading id="stats-web-performance" tone="running">
        Web performance
      </SectionHeading>
      <Box
        data-mock-part="section-head"
        display="flex"
        alignItems="center"
        justifyContent="space-between"
        gap="4"
        flexWrap="wrap"
        mb="3"
      >
        <Text fontSize="xs" color="text.muted">
          All projects, ignores the project filter · p75 · samples from this browser build and
          others
        </Text>
        <chakra.label display="flex" alignItems="center" gap="2" fontSize="sm" ml="auto">
          <input
            type="checkbox"
            checked={includeDev}
            onChange={(e) => onIncludeDev(e.target.checked)}
          />
          Include dev
        </chakra.label>
      </Box>
      {empty ? (
        <Box data-testid="perf-empty" display="flex" flexDirection="column" gap="1">
          <Text color="text.muted">No web performance samples in this range.</Text>
          {data.dev_hidden && <Text color="text.muted">Dev samples are hidden.</Text>}
        </Box>
      ) : (
        <>
          <Box
            display="flex"
            gap="2"
            flexWrap="wrap"
            mb="3"
            role="group"
            aria-label="Web performance tab"
          >
            {TABS.map((t) => (
              <Button
                key={t.id}
                size="sm"
                emphasis={tab === t.id ? "secondary" : "ghost"}
                tone="muted"
                aria-pressed={tab === t.id}
                onClick={() => onTab(t.id)}
              >
                {t.label}
              </Button>
            ))}
          </Box>
          <Box
            data-mock-part="metric-cards"
            display="grid"
            gridTemplateColumns="repeat(auto-fill, minmax(min(210px, 100%), 1fr))"
            gap="3"
          >
            {cards.map((c) => (
              <MetricCard key={c.metric} card={c} range={range} />
            ))}
          </Box>
        </>
      )}
    </Box>
  )
}
