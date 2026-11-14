import { Box, chakra, Flex, Grid, Text } from "@chakra-ui/react"
import type { SessionUsage } from "../generated/server"

const number = new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 })
const money = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" })

function Reading({ label, value }: { label: string; value?: number }) {
  return value == null ? null : (
    <Flex justify="space-between" gap="3">
      <Text color="text.dim">{label}</Text>
      <Text fontFamily="mono">{number.format(value)}</Text>
    </Flex>
  )
}

export function ContextDetails({
  usage,
  used,
  ceiling,
}: {
  usage?: SessionUsage
  used: number
  ceiling: number
}) {
  return (
    <Box mt="3">
      <Reading label="First request" value={usage?.first_context} />
      <Reading label="Peak request" value={usage?.peak_context} />
      <Reading label="Remaining to smart zone" value={Math.max(0, ceiling - used)} />
      <Reading label="Reported model capacity" value={usage?.model_context_window} />
      {(usage?.compactions ?? 0) > 0 ? (
        <Text color="text.dim" mt="1">
          {usage!.compactions} {usage!.compactions === 1 ? "compaction" : "compactions"}
        </Text>
      ) : null}
      {usage?.context_stale ? (
        <Text color="tone.waiting.fg" mt="1" role="status">
          Compacted. Waiting for a new request reading.
        </Text>
      ) : null}
      {usage?.first_context ? (
        <Text color="text.dim" mt="2">
          First request includes instructions, tools, the prompt, and any inherited history.
        </Text>
      ) : null}
    </Box>
  )
}

export function UsageDetails({ usage }: { usage: SessionUsage }) {
  const totals = usage.totals
  return (
    <Box borderTopWidth="1px" borderColor="border" mt="3" pt="2">
      <Flex gap="2" justify="space-between" align="baseline">
        <Text>
          <chakra.span fontFamily="mono" fontWeight="700">
            {number.format(usage.total_tokens)}
          </chakra.span>{" "}
          <chakra.span color="text.dim">total tokens</chakra.span>
        </Text>
        {usage.estimated_cost != null ? (
          <Text color="text.dim">≈{money.format(usage.estimated_cost)}</Text>
        ) : null}
      </Flex>
      {usage.scope ? (
        <Text color="text.dim" mt="1">
          {usage.scope === "conversation"
            ? "Conversation, including subagents"
            : "This thread only"}
        </Text>
      ) : null}
      {totals ? (
        <Grid templateColumns="minmax(0, 1fr)" gap="1" mt="2">
          <Reading label="Uncached input" value={totals.input_tokens} />
          <Reading label="Cache reads" value={totals.cache_read_tokens} />
          <Reading label="Cache writes" value={totals.cache_creation_tokens} />
          <Reading label="Output" value={totals.output_tokens} />
          <Reading label="Reasoning, included in output" value={totals.reasoning_tokens} />
        </Grid>
      ) : null}
      {usage.main_tokens != null && usage.subagent_tokens != null ? (
        <Box mt="2">
          <Reading label="Main agent" value={usage.main_tokens} />
          <Reading label="Subagents" value={usage.subagent_tokens} />
        </Box>
      ) : null}
      <Text color="text.dim" mt="2">
        Repeated input counts on each request. Readings update after requests are recorded.
      </Text>
    </Box>
  )
}
