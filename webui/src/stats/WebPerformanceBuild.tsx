import { Box, chakra, Text } from "@chakra-ui/react"
import type {
  StatsWebPerfBuild,
  StatsWebPerfBuildStrip,
  StatsWebPerfPoorINP,
  StatsWebPerfWhySlow,
} from "../generated/server"
import { sizeText } from "../health/formatBytes"
import { Collapsible } from "../ticket/Collapsible"

const MIN_SAMPLES = 20

const ROWS: { metric: string; label: string }[] = [
  { metric: "ttfb", label: "TTFB" },
  { metric: "api_latency", label: "Blocking API latency" },
  { metric: "js_bytes", label: "JS transferred" },
  { metric: "css_bytes", label: "CSS transferred" },
]

// Mock look: padded cells, a rule under every row, muted headers.
const CELLS = {
  "& td, & th": { padding: "6px 8px", borderBottomWidth: "1px" },
  "& th": { fontWeight: "400", color: "var(--chakra-colors-text-muted)" },
}

const isBytes = (metric: string) => metric.endsWith("_bytes")

function formatWhy(metric: string, v: number | undefined): string {
  if (v == null) return "—"
  return isBytes(metric) ? sizeText(v, "decimal") : `${Math.round(v)} ms`
}

// build_id looks like "b-7c2e55"; the label shows the first four hash characters.
function buildLabel(b: StatsWebPerfBuild): string {
  return `${b.web_version} · ${b.build_id.replace(/^b-/, "").slice(0, 4)}`
}

function Pct({ pct }: { pct: number | undefined }) {
  if (pct == null) return <b>—</b>
  const rounded = Math.round(pct)
  const color = rounded < 0 ? "tone.done.fg" : rounded > 0 ? "tone.deadlocked.fg" : "text.muted"
  return (
    <chakra.b color={color}>
      {rounded > 0 ? "+" : ""}
      {rounded}%
    </chakra.b>
  )
}

function BuildName({ title, build }: { title: string; build: StatsWebPerfBuild }) {
  return (
    <Box>
      <Text fontSize="xs" color="text.muted">
        {title}
      </Text>
      <Text fontFamily="mono" fontWeight="600">
        {buildLabel(build)}
      </Text>
    </Box>
  )
}

export function BuildStrip({ strip }: { strip: StatsWebPerfBuildStrip }) {
  return (
    <Box
      data-mock-part="build-compare"
      data-testid="perf-build-strip"
      borderWidth="1px"
      borderRadius="md"
      p="3"
      mb="3"
      display="flex"
      gap="4"
      flexWrap="wrap"
      alignItems="center"
    >
      <BuildName title="Current build" build={strip.current} />
      {strip.previous ? (
        <>
          <BuildName title="Compared with" build={strip.previous} />
          <Text fontSize="sm" color="text.muted">
            LCP p75 <Pct pct={strip.lcp_change_pct} /> · INP <Pct pct={strip.inp_change_pct} /> · JS
            transfer <Pct pct={strip.js_change_pct} />
          </Text>
        </>
      ) : (
        <Text fontSize="sm" color="text.muted">
          no earlier build in range
        </Text>
      )}
    </Box>
  )
}

function WhyRow({
  label,
  row,
  hasPrevious,
}: {
  label: string
  row: StatsWebPerfWhySlow
  hasPrevious: boolean
}) {
  const low = row.count < MIN_SAMPLES
  return (
    <chakra.tr
      data-testid={`why-row-${row.metric}`}
      data-low={low || undefined}
      opacity={low ? 0.55 : 1}
    >
      <td>{label}</td>
      <chakra.td textAlign="right">{low ? "not enough" : formatWhy(row.metric, row.p75)}</chakra.td>
      <chakra.td textAlign="right">
        {hasPrevious && !low && row.change_pct != null
          ? formatWhy(row.metric, row.previous_p75)
          : "—"}
      </chakra.td>
    </chakra.tr>
  )
}

function PoorInp({ poor }: { poor: StatsWebPerfPoorINP }) {
  const low = poor.count < MIN_SAMPLES
  const phases: [string, number | undefined][] = [
    ["Input delay", poor.input_delay_p75],
    ["Processing", poor.processing_p75],
    ["Presentation delay", poor.presentation_delay_p75],
  ]
  return (
    <Box data-testid="perf-poor-inp" data-low={low || undefined} mt="4">
      <Text fontWeight="600">Poor INP samples: phase p75 ({poor.count} samples)</Text>
      <Box opacity={low ? 0.55 : 1}>
        {low ? (
          <Text color="text.muted">not enough</Text>
        ) : (
          <chakra.table w="100%" fontSize="sm" css={CELLS}>
            <tbody>
              {phases.map(([label, v]) => (
                <tr key={label}>
                  <td>{label}</td>
                  <chakra.td textAlign="right">{formatWhy("ms", v)}</chakra.td>
                </tr>
              ))}
              {poor.top_actions.slice(0, 3).map((a) => (
                <tr key={a.action}>
                  <td>Slow action: {a.action}</td>
                  <chakra.td textAlign="right">{a.count} poor</chakra.td>
                </tr>
              ))}
              <tr>
                <td>Worst-frame chunk</td>
                <chakra.td textAlign="right">{poor.worst_chunk ?? "—"}</chakra.td>
              </tr>
            </tbody>
          </chakra.table>
        )}
      </Box>
    </Box>
  )
}

export function WhySlow({
  rows,
  poor,
  hasPrevious,
}: {
  rows: StatsWebPerfWhySlow[]
  poor: StatsWebPerfPoorINP
  hasPrevious: boolean
}) {
  return (
    <Box data-mock-part="why-slow" borderWidth="1px" borderRadius="md" p="3" mt="3">
      <Collapsible label="Why slow? Diagnostics">
        <chakra.table w="100%" fontSize="sm" css={CELLS}>
          <thead>
            <tr>
              <chakra.th textAlign="left">Diagnostic</chakra.th>
              <chakra.th textAlign="right">p75 now</chakra.th>
              <chakra.th textAlign="right">previous build</chakra.th>
            </tr>
          </thead>
          <tbody>
            {ROWS.map(({ metric, label }) => {
              const row = rows.find((r) => r.metric === metric) ?? { metric, count: 0 }
              return <WhyRow key={metric} label={label} row={row} hasPrevious={hasPrevious} />
            })}
          </tbody>
        </chakra.table>
        <PoorInp poor={poor} />
      </Collapsible>
    </Box>
  )
}
