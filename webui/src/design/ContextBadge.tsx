import { Box, chakra, Flex, Grid, Popover, Portal, Text } from "@chakra-ui/react"
import type { SessionInfo, SessionUsage } from "../generated/server"
import type { Tone } from "../ui/tone"
import { ContextDetails, UsageDetails } from "./ContextDetails"

type ContextLevel = NonNullable<SessionInfo["context_level"]>

const LEVEL_TONE: Record<ContextLevel, Tone> = {
  green: "done",
  yellow: "waiting",
  orange: "paused",
  red: "deadlocked",
}

const compact = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 })
const kilo = (n: number) => `${Math.round(n / 1000)}K`
const tokens = (n: number) => compact.format(n)

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

function ContextSplit({
  usage,
  ceiling,
  used,
}: {
  usage?: SessionUsage
  ceiling: number
  used: number
}) {
  const input = usage?.input_tokens ?? 0
  const cacheRead = usage?.cache_read_tokens ?? 0
  const cacheCreation = usage?.cache_creation_tokens ?? 0
  const split = input + cacheRead + cacheCreation > 0
  const parts = [
    { label: "Reused cache", value: cacheRead, color: "chart.blue" },
    { label: "New input", value: input, color: "chart.purple" },
    { label: "Newly cached", value: cacheCreation, color: "chart.amber" },
  ]
  const barParts = split ? parts : [{ label: "Used", value: used, color: "chart.blue" }]
  return (
    <>
      <Flex h="2" overflow="hidden" borderRadius="full" bg="hairline" aria-label="Context split">
        {barParts.map((part) => (
          <Box
            key={part.label}
            bg={part.color}
            w={`${Math.min(100, (part.value / ceiling) * 100)}%`}
            data-context-part={part.label}
          />
        ))}
      </Flex>
      {split ? (
        <Grid templateColumns="repeat(3, minmax(0, 1fr))" gap="2" mt="2">
          {parts.map((part) => (
            <Box key={part.label} minW="0">
              <Text fontFamily="mono" fontWeight="600">
                {tokens(part.value)}
              </Text>
              <Text color="text.dim" lineClamp={1} title={part.label}>
                {part.label}
              </Text>
            </Box>
          ))}
        </Grid>
      ) : null}
    </>
  )
}

export function ContextBadge({
  tokens: used,
  level,
  ceiling,
  usage,
}: {
  tokens: number
  level: ContextLevel
  ceiling: number
  usage?: SessionUsage
}) {
  const tone = LEVEL_TONE[level]
  const fill = Math.min(1, Math.max(0, used / ceiling))
  const percent = Math.round((used / ceiling) * 100)
  return (
    <Popover.Root
      positioning={{ placement: "top-end", gutter: 8, fitViewport: true, overflowPadding: 8 }}
      lazyMount
      unmountOnExit
    >
      <Popover.Trigger asChild>
        <chakra.button
          type="button"
          aria-label="Context"
          data-tone={tone}
          display="inline-flex"
          alignItems="center"
          gap="1"
          flexShrink={0}
          borderWidth="0"
          color={`tone.${tone}.fg`}
          fontSize="xs"
          lineHeight="1"
          cursor="pointer"
          style={{ fontVariantNumeric: "tabular-nums" }}
        >
          <Ring fill={fill} tone={tone} />
          {kilo(used)}
        </chakra.button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            role="dialog"
            aria-label="Context usage"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="md"
            boxShadow="lg"
            color="text"
            boxSizing="border-box"
            w="80"
            maxW="calc(100vw - 2rem)"
            maxH="min(calc(100dvh - 2rem), var(--available-height))"
            overflowY="auto"
            p="3"
            fontSize="xs"
          >
            <Flex justify="space-between" align="baseline">
              <Text fontWeight="700">Latest request input</Text>
              <Text color={`tone.${tone}.fg`} fontFamily="mono" fontWeight="600">
                {percent}%
              </Text>
            </Flex>
            <Flex align="baseline" gap="1" mb="2" whiteSpace="nowrap">
              <Text fontFamily="mono" fontWeight="700" fontSize="lg">
                {kilo(used)}
              </Text>
              <Text color="text.dim">of {kilo(ceiling)} smart zone</Text>
            </Flex>
            <ContextSplit usage={usage} ceiling={ceiling} used={used} />
            <ContextDetails usage={usage} used={used} ceiling={ceiling} />
            {usage ? <UsageDetails usage={usage} /> : null}
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}
