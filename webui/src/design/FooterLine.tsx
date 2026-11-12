import { Box, chakra } from "@chakra-ui/react"
import { InfoTip } from "../ui/info-tip/InfoTip"
import type { Tone } from "../ui/tone"
import { useWide } from "../useWide"
import type { SessionInfo } from "../generated/server"
import { agentDetails } from "./agentDetails"

// The server decides the level from the count; the web only picks a colour for it.
const LEVEL_TONE: Record<string, Tone> = {
  green: "done",
  yellow: "waiting",
  orange: "paused",
  red: "deadlocked",
}

const kilo = (n: number) => `${Math.round(n / 1000)}K`

const RING = 14
const STROKE = 2.5

function Ring({ fill, tone }: { fill: number; tone: Tone }) {
  const r = (RING - STROKE) / 2
  const c = RING / 2
  return (
    <chakra.svg
      viewBox={`0 0 ${RING} ${RING}`}
      w={`${RING}px`}
      h={`${RING}px`}
      fill="none"
      strokeWidth={STROKE}
      aria-hidden
    >
      <g transform={`rotate(-90 ${c} ${c})`}>
        <chakra.circle cx={c} cy={c} r={r} stroke="border" />
        <chakra.circle
          data-testid="context-ring"
          data-fill={fill}
          cx={c}
          cy={c}
          r={r}
          stroke={`tone.${tone}.fg`}
          pathLength={100}
          strokeDasharray={`${fill * 100} 100`}
          display={fill > 0 ? undefined : "none"}
        />
      </g>
    </chakra.svg>
  )
}

/** Context count against the smart-zone ceiling, as a gauge ring and thousands, with the tooltip. */
export function ContextBadge({
  tokens,
  level,
  ceiling,
}: {
  tokens: number
  level?: string
  ceiling: number
}) {
  const tone = LEVEL_TONE[level ?? ""] ?? "muted"
  const fill = Math.min(1, Math.max(0, tokens / ceiling))
  return (
    <InfoTip
      label="Context"
      variant="chip"
      outline={false}
      chipColor={`tone.${tone}.fg`}
      content={`${kilo(tokens)} of ${kilo(ceiling)} smart zone`}
    >
      <chakra.span display="inline-flex" alignItems="center" gap="1" data-tone={tone}>
        <Ring fill={fill} tone={tone} />
        {kilo(tokens)}
      </chakra.span>
    </InfoTip>
  )
}

/** The line inside a composer's box: the model on the left, the context badge on the right. */
export function FooterLine({
  session,
  ceiling,
  modelOnly,
}: {
  session: SessionInfo
  ceiling?: number
  modelOnly?: boolean
}) {
  const wide = useWide()
  const { model, effort } = agentDetails(session)
  const text = wide && effort ? `${model} · ${effort}` : model
  const showBadge = session.context_tokens != null && !!ceiling && !modelOnly
  return (
    <Box
      data-testid="footer-line"
      display="flex"
      alignItems="center"
      justifyContent="space-between"
      gap="2"
      fontSize="xs"
      color="text.dim"
    >
      <chakra.span truncate>{text}</chakra.span>
      {showBadge && (
        <ContextBadge
          tokens={session.context_tokens!}
          level={session.context_level}
          ceiling={ceiling!}
        />
      )}
    </Box>
  )
}
