import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import type { StatsWebPerfRoute, StatsWebPerfTrend } from "../generated/server"
import { Button } from "../ui/button/Button"

export const MIN_SAMPLES = 20

export const TREND_METRICS: { id: string; label: string }[] = [
  { id: "lcp", label: "LCP" },
  { id: "inp", label: "INP" },
  { id: "cls", label: "CLS" },
  { id: "fcp", label: "FCP" },
  { id: "app_ready", label: "App ready" },
  { id: "route_ready", label: "Route ready" },
]

const ACCENT = "var(--chakra-colors-tone-running-fg, currentColor)"
const W = 640
const H = 190
const LEFT = 56
const RIGHT = 630
const TOP = 16
const BOTTOM = 166

function formatTick(metric: string, v: number): string {
  if (metric === "cls") return v.toFixed(2)
  return v >= 1000 ? `${+(v / 1000).toFixed(2)} s` : `${Math.round(v)} ms`
}

// Two even gaps of a round size (1, 2, 5 × 10^n) that cover [lo, hi].
function niceRange(lo: number, hi: number): { min: number; max: number; ticks: number[] } {
  const span = Math.max(hi - lo, Math.abs(hi) * 0.05, 1e-6)
  const raw = span / 2
  const mag = 10 ** Math.floor(Math.log10(raw))
  const step = ([1, 2, 5, 10].find((m) => m * mag >= raw) ?? 10) * mag
  const min = Math.floor(lo / step) * step
  const gaps = min + 2 * step < hi ? 3 : 2
  const ticks = Array.from({ length: gaps + 1 }, (_, i) => +(min + i * step).toPrecision(12))
  return { min: ticks[0], max: ticks[gaps], ticks }
}

function dayLabel(day: string): string {
  const d = new Date(`${day}T00:00:00Z`)
  if (Number.isNaN(d.getTime())) return day
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" })
}

function Trend({
  metric,
  label,
  trend,
  days,
}: {
  metric: string
  label: string
  trend?: StatsWebPerfTrend
  days: string[]
}) {
  const points = trend?.days ?? []
  const values = points.flatMap((p) => (p.p75 == null ? [] : [p.p75]))
  const n = points.length
  const x = (i: number) => (n <= 1 ? RIGHT : LEFT + ((RIGHT - LEFT) * i) / (n - 1))
  const lo = values.length ? Math.min(...values) : 0
  const hi = values.length ? Math.max(...values) : 1
  const { min, max, ticks } = niceRange(lo, hi)
  const y = (v: number) => BOTTOM - ((v - min) / (max - min)) * (BOTTOM - TOP)
  const step = n <= 1 ? 0 : (RIGHT - LEFT) / (n - 1)
  const low = points.flatMap((p, i) => (p.count < MIN_SAMPLES ? [i] : []))
  const line = points.flatMap((p, i) => (p.p75 == null ? [] : [`${x(i)},${y(p.p75)}`]))
  const last = points.length - 1
  return (
    <chakra.svg
      viewBox={`0 0 ${W} ${H}`}
      role="img"
      aria-label={`daily p75 ${label}`}
      w="100%"
      color="text.muted"
    >
      {low.map((i) => (
        <rect
          key={i}
          data-testid="trend-low-day"
          x={x(i) - step / 2}
          y={10}
          width={Math.max(step, 4)}
          height={BOTTOM - 10}
          fill="currentColor"
          opacity={0.12}
        />
      ))}
      {low.length > 0 && (
        <text x={LEFT + 4} y={22} fontSize="11" fill="currentColor">
          fewer than 20 samples
        </text>
      )}
      {ticks.map((v) => (
        <g key={v}>
          <line x1={LEFT} x2={RIGHT} y1={y(v)} y2={y(v)} stroke="currentColor" opacity={0.25} />
          <text x={LEFT - 6} y={y(v) + 3} fontSize="11" textAnchor="end" fill="currentColor">
            {formatTick(metric, v)}
          </text>
        </g>
      ))}
      {line.length > 1 && (
        <polyline points={line.join(" ")} fill="none" stroke={ACCENT} strokeWidth="2" />
      )}
      {points.map((p, i) =>
        p.p75 == null ? null : (
          <circle
            key={i}
            data-testid="trend-point"
            cx={x(i)}
            cy={y(p.p75)}
            r={i === last ? 4 : 2}
            fill={ACCENT}
          />
        ),
      )}
      {days.length > 0 && (
        <>
          <text x={LEFT} y={184} fontSize="11" fill="currentColor">
            {dayLabel(days[0])}
          </text>
          <text x={RIGHT} y={184} fontSize="11" textAnchor="end" fill="currentColor">
            {dayLabel(days[days.length - 1])}
          </text>
        </>
      )}
    </chakra.svg>
  )
}

export function WebPerformanceTrend({
  trends,
  days,
  routes,
}: {
  trends: StatsWebPerfTrend[]
  days: string[]
  routes: StatsWebPerfRoute[]
}) {
  const [metric, setMetric] = useState("lcp")
  const label = TREND_METRICS.find((m) => m.id === metric)?.label ?? metric
  const top = [...routes].sort((a, b) => b.p75 - a.p75).slice(0, 5)
  return (
    <Box
      data-mock-part="trend-and-routes"
      display="grid"
      gridTemplateColumns={{ base: "1fr", md: "2fr 1fr" }}
      gap="3"
      mt="3"
    >
      <Box borderWidth="1px" borderRadius="md" p="3">
        <Box display="flex" justifyContent="space-between" gap="2" flexWrap="wrap" mb="2">
          <Text fontWeight="600">Daily p75</Text>
          <Box display="flex" gap="1" flexWrap="wrap" role="group" aria-label="Trend metric">
            {TREND_METRICS.map((m) => (
              <Button
                key={m.id}
                size="sm"
                emphasis={metric === m.id ? "secondary" : "ghost"}
                tone="muted"
                aria-pressed={metric === m.id}
                onClick={() => setMetric(m.id)}
              >
                {m.label}
              </Button>
            ))}
          </Box>
        </Box>
        <Trend
          metric={metric}
          label={label}
          trend={trends.find((t) => t.metric === metric)}
          days={days}
        />
      </Box>
      <Box borderWidth="1px" borderRadius="md" p="3" data-testid="slowest-routes">
        <Text fontWeight="600">Slowest routes</Text>
        <Text fontSize="xs" color="text.muted" mb="2">
          route changes, by p75
        </Text>
        {top.length === 0 ? (
          <Text color="text.muted">—</Text>
        ) : (
          <chakra.table w="100%" fontSize="sm">
            <thead>
              <tr>
                <chakra.th textAlign="left">Route</chakra.th>
                <chakra.th textAlign="right">p75 route ready</chakra.th>
                <chakra.th textAlign="right">samples</chakra.th>
              </tr>
            </thead>
            <tbody>
              {top.map((r) => {
                const lowRow = r.count < MIN_SAMPLES
                return (
                  <chakra.tr
                    key={r.route}
                    data-testid={`route-row-${r.route}`}
                    data-low={lowRow || undefined}
                    opacity={lowRow ? 0.55 : 1}
                    borderTopWidth="1px"
                  >
                    <chakra.td fontFamily="mono" py="1.5" px="2">
                      {r.route}
                    </chakra.td>
                    <chakra.td textAlign="right" py="1.5" px="2">
                      {formatTick("route_ready", r.p75)}
                    </chakra.td>
                    <chakra.td textAlign="right" py="1.5" px="2">
                      {lowRow ? `${r.count} · not enough` : r.count}
                    </chakra.td>
                  </chakra.tr>
                )
              })}
            </tbody>
          </chakra.table>
        )}
      </Box>
    </Box>
  )
}
