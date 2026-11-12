import { Box, chakra, Text } from "@chakra-ui/react"
import type { ScheduleRun } from "../generated/server"
import { ticketHref } from "../hrefs"
import type { Tone } from "../ui/tone"
import { formatNextRun } from "./format"

const STATUS: Record<string, { word: string; tone: Tone }> = {
  finished: { word: "Finished", tone: "done" },
  "ran-late": { word: "Ran late", tone: "paused" },
  skipped: { word: "Skipped", tone: "muted" },
  paused: { word: "Paused", tone: "waiting" },
  running: { word: "Running", tone: "running" },
  queued: { word: "Queued", tone: "muted" },
  cancelled: { word: "Cancelled", tone: "muted" },
}

export function LastRunRow({ run }: { run: ScheduleRun }) {
  const status = STATUS[run.status] ?? {
    word: run.status,
    tone: "muted" as Tone,
  }
  const skipped = run.status === "skipped"
  const text = skipped ? run.reason_text || run.reason : run.result
  return (
    <Box
      as="li"
      listStyleType="none"
      display="flex"
      alignItems="baseline"
      gap="3"
      py="2"
      borderBottomWidth="1px"
      borderColor="border"
    >
      <Text flex="none" w="110px" color="text.muted" fontSize="14px">
        {run.scheduled_for ? formatNextRun(run.scheduled_for) : "Run now"}
      </Text>
      {run.address && (
        <chakra.a href={ticketHref(run.address)} flex="none" fontSize="14px">
          {run.address}
        </chakra.a>
      )}
      <Text flex="1" minW="0" fontSize="14px" truncate>
        {text}
        {skipped && run.attention_address && (
          <>
            {" "}
            <chakra.a href={ticketHref(run.attention_address)}>{run.attention_address}</chakra.a>
          </>
        )}
      </Text>
      <Text
        as="span"
        flex="none"
        fontSize="14px"
        data-tone={status.tone}
        color={`tone.${status.tone}.fg`}
      >
        {status.word}
      </Text>
    </Box>
  )
}

export function LastRuns({ runs }: { runs: ScheduleRun[] }) {
  return (
    <Box
      data-mock-part="row-last-runs"
      display="grid"
      gridTemplateColumns={{ base: "1fr", md: "120px 1fr" }}
      gap="2"
      py="4"
      borderBottomWidth="1px"
      borderColor="border"
    >
      <Text color="text.muted" fontSize="14px">
        Last runs
      </Text>
      <Box minW="0">
        <Box as="ul" data-mock-part="last-runs" m="0" p="0">
          {runs.map((r, i) => (
            <LastRunRow key={`${r.address}|${r.scheduled_for}|${i}`} run={r} />
          ))}
        </Box>
        <Text color="text.muted" fontSize="13px" mt="1">
          Last 10 runs.
        </Text>
      </Box>
    </Box>
  )
}
