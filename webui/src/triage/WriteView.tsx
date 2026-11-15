import { Box, Text } from "@chakra-ui/react"
import { postKeepTriageRow, postRetryTriageRow, type TriageRowAction } from "./api"
import { useState } from "react"
import type { TriageMove, TriageRow, TriageView } from "../generated/server"
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

const moves = (view: TriageView): TriageMove[] => view.summary?.moves ?? []

/** Rows plus move units the write has finished with: written, gone or kept. */
export const rowsWritten = (view: TriageView) =>
  view.rows.filter((r) => r.write_state === "done" || r.write_state === "gone").length +
  moves(view).filter((m) => m.state === "done" || m.state === "kept").length

export const rowsTotal = (view: TriageView) => view.rows.length + moves(view).length

/** "x of y rows written", the Progress sub-row text and the card header's count. */
export const writtenLine = (view: TriageView) =>
  `${rowsWritten(view)} of ${rowsTotal(view)} rows written`

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
      {row.new_addresses?.map((a) => (
        <Text key={a} color="text.dim">
          → {a}
        </Text>
      ))}
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
      flexWrap="wrap"
      alignItems="center"
      justifyContent="space-between"
      gap="2"
      py="2"
      borderBottomWidth="1px"
      borderColor="border"
    >
      <Box display="flex" flexDirection="column" minW="0" flex="1 1 200px">
        <Box display="flex" alignItems="baseline" gap="2" flexWrap="wrap">
          <Text color="tone.deadlocked.fg" aria-label="failed">
            ×
          </Text>
          <Text fontFamily="mono" title={row.title}>
            {row.address}
          </Text>
          {row.new_addresses?.map((a) => (
            <Text key={a} color="text.dim">
              → {a}
            </Text>
          ))}
        </Box>
        {row.refusal && (
          <Text fontSize="sm" color="text.muted" overflowWrap="anywhere">
            {row.refusal}
          </Text>
        )}
      </Box>
      <Box display="flex" gap="2">
        {row.can_retry && (
          <Button size="sm" emphasis="secondary" disabled={pending} onClick={() => act(retry)}>
            Retry
          </Button>
        )}
        {row.can_keep && (
          <Button size="sm" emphasis="secondary" disabled={pending} onClick={() => act(keep)}>
            Keep for later
          </Button>
        )}
      </Box>
    </Box>
  )
}

/** An old-epic move unit as a row of the list. It has no Retry or Keep for later: the wire has no unit calls yet. */
function MoveRow({ move }: { move: TriageMove }) {
  const failed = move.state === "failed"
  return (
    <Box display="flex" flexDirection="column" py="2" borderBottomWidth="1px" borderColor="border">
      <Box display="flex" alignItems="center" gap="2" flexWrap="wrap">
        <Chip tone={failed ? "deadlocked" : move.state === "done" ? "done" : "muted"}>
          {move.state}
        </Chip>
        <Text>
          move {move.old_epic} -&gt; {move.area}
        </Text>
      </Box>
      {failed && move.last_error && (
        <Text fontFamily="mono" color="tone.deadlocked.fg">
          {move.last_error}
        </Text>
      )}
      {move.state === "kept" && (
        <Text fontSize="sm" color="text.muted">
          kept for later
        </Text>
      )}
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
  const [open, setOpen] = useState(false)
  const isWritten = (r: TriageRow) => r.write_state === "done" || r.write_state === "gone"
  const isFailed = (r: TriageRow) => r.write_state === "failed"
  const unitList = moves(view)
  const writtenRows = view.rows.filter(isWritten)
  const writtenUnits = unitList.filter((m) => m.state === "done")
  const folded = writtenRows.length + writtenUnits.length
  const rowEl = (row: TriageRow) => (
    <WriteRow
      key={row.address}
      project={project}
      revision={view.revision ?? 0}
      row={row}
      retry={retry}
      keep={keep}
      refetch={refetch}
    />
  )
  const unitEl = (m: TriageMove) => <MoveRow key={`${m.old_epic}:${m.area}`} move={m} />
  return (
    <Box
      display="flex"
      flexDirection="column"
      p="3"
      borderWidth="1px"
      borderColor="border"
      borderRadius="xl"
      bg="surface.raised"
    >
      <Box display="flex" alignItems="center" gap="3" pb="2">
        <Text fontWeight="700" whiteSpace="nowrap">
          Write · {rowsWritten(view)} of {rowsTotal(view)} rows
        </Text>
        <Box flex="1" h="2" borderRadius="full" bg="border" overflow="hidden">
          <Box
            h="100%"
            bg="tone.done.fg"
            style={{ width: `${(100 * rowsWritten(view)) / Math.max(rowsTotal(view), 1)}%` }}
          />
        </Box>
      </Box>
      {view.rows.filter(isFailed).map(rowEl)}
      {unitList.filter((m) => m.state === "failed").map(unitEl)}
      {view.rows.filter((r) => !isFailed(r) && !isWritten(r)).map(rowEl)}
      {unitList.filter((m) => m.state !== "failed" && m.state !== "done").map(unitEl)}
      {folded > 0 && (
        <>
          <Button
            emphasis="ghost"
            tone="muted"
            alignSelf="flex-start"
            onClick={() => setOpen(!open)}
            aria-expanded={open}
          >
            <Text as="span" color="tone.done.fg" mr="2" aria-hidden="true">
              ✓
            </Text>
            {folded} more rows written
          </Button>
          {open && (
            <>
              {writtenRows.map(rowEl)}
              {writtenUnits.map(unitEl)}
            </>
          )}
        </>
      )}
      <Text fontSize="xs" color="text.dim" pt="2">
        Failed rows do not stop the others. Chain edges and the code-review stub are written last.
      </Text>
    </Box>
  )
}
