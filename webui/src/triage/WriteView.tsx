import { Box, Text } from "@chakra-ui/react"
import { postKeepTriageRow, postRetryTriageRow, type TriageRowAction } from "./api"
import type { TriageRow, TriageView } from "../generated/server"
import { Button } from "../ui/button/Button"
import { Chip } from "../ui/chip/Chip"
import type { Tone } from "../ui/tone"
import { useWrite } from "../useWrite"

const TONES: Record<string, Tone> = {
  done: "done",
  running: "running",
  failed: "deadlocked",
  pending: "muted",
  gone: "muted",
}

/** Rows the write has finished with, for the strip: written or already gone. */
export const rowsWritten = (view: TriageView) =>
  view.rows.filter((r) => r.write_state === "done" || r.write_state === "gone").length

/** What was done to a written row, from the plan the person approved. */
function summaryOf(row: TriageRow): string {
  if (row.write_state === "gone") return "gone"
  if (row.write_state !== "done") return ""
  switch (row.action) {
    case "close":
      return row.destination ? `closed, ${row.destination}` : "closed"
    case "merge":
      return "merged"
    case "keep":
      return "kept for later"
    default:
      return "moved"
  }
}

function WriteRow({
  project,
  revision,
  row,
  retry,
  keep,
  refetch,
}: {
  project: string
  revision: number
  row: TriageRow
  retry: TriageRowAction
  keep: TriageRowAction
  refetch: () => void
}) {
  const { run, pending } = useWrite()
  const act = (call: TriageRowAction) =>
    run(() => call({ project, revision, address: row.address }), { onDone: refetch })
  const state = row.write_state ?? "pending"
  const summary = summaryOf(row)
  const failed = state === "failed"
  const name = (
    <Box display="flex" alignItems="center" gap="2" flexWrap="wrap">
      {!failed && <Chip tone={TONES[state] ?? "muted"}>{state}</Chip>}
      <Text fontWeight={failed ? "700" : undefined} title={row.title}>
        {row.address}
      </Text>
      {failed && <Chip tone={TONES[state]}>{state}</Chip>}
      {row.new_address && <Text color="text.dim">→ {row.new_address}</Text>}
    </Box>
  )
  if (!failed)
    return (
      <Box
        display="flex"
        flexDirection="column"
        py="2"
        borderBottomWidth="1px"
        borderColor="border"
      >
        {name}
        {summary && (
          <Text fontSize="sm" color="text.muted">
            {summary}
          </Text>
        )}
      </Box>
    )
  return (
    <Box
      display="flex"
      flexDirection="column"
      gap="1"
      p="3"
      my="1"
      borderWidth="1px"
      borderColor="tone.deadlocked.fg"
      bg="tone.deadlocked.bg"
      borderRadius="md"
    >
      {name}
      {row.refusal && (
        <Text fontFamily="mono" color="tone.deadlocked.fg">
          {row.refusal}
        </Text>
      )}
      <Box display="flex" gap="2">
        {row.can_retry && (
          <Button disabled={pending} onClick={() => act(retry)}>
            Retry
          </Button>
        )}
        {row.can_keep && (
          <Button emphasis="ghost" disabled={pending} onClick={() => act(keep)}>
            Keep for later
          </Button>
        )}
      </Box>
    </Box>
  )
}

/** The Write step: one row per follow-up with its state and result; a failed row names the refusal and offers what the payload allows. */
export function WriteView({
  project,
  view,
  refetch,
  retry = postRetryTriageRow,
  keep = postKeepTriageRow,
}: {
  project: string
  view: TriageView
  refetch: () => void
  retry?: TriageRowAction
  keep?: TriageRowAction
}) {
  return (
    <Box display="flex" flexDirection="column">
      {view.rows.map((row) => (
        <WriteRow
          key={row.address}
          project={project}
          revision={view.revision ?? 0}
          row={row}
          retry={retry}
          keep={keep}
          refetch={refetch}
        />
      ))}
      <Text fontSize="xs" color="text.dim" pt="2">
        Failed rows do not stop the others. Chain edges and the code-review stub are written last.
      </Text>
    </Box>
  )
}
