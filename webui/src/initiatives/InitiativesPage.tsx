import { Box, Link, Text } from "@chakra-ui/react"
import { useConnectionContext } from "../connection"
import type { InitiativeRow } from "../generated/server"
import { initiativeHref } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { Button } from "../ui/button/Button"
import { Chip } from "../ui/chip/Chip"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import type { Tone } from "../ui/tone"
import { useFetched } from "../useFetched"
import { useFetchedReady } from "../useReady"
import { useActiveWorkspace } from "../workspaces/useActiveWorkspace"
import { fetchInitiatives, type FetchInitiatives } from "./initiatives"

const STATE: Record<string, { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "waiting" },
  stopped: { label: "Stopped", tone: "deadlocked" },
  finished: { label: "Finished", tone: "done" },
}

/** The Current cell's text; the server derives every part of it. */
export function currentText(r: InitiativeRow): string {
  if (r.state === "stopped") return `Stopped at Milestone ${r.stopped_at} gate`
  if (r.state === "finished") return "All Milestones gated"
  return r.current ? `Milestone ${r.current_number}: ${r.current} (${r.current_state})` : ""
}

const headCell = {
  textAlign: "left",
  fontWeight: "400",
  fontSize: "13px",
  color: "text.muted",
  py: "2",
  pr: "4",
  borderBottomWidth: "1px",
  borderColor: "hairline",
} as const

function InitiativeTableRow({ row }: { row: InitiativeRow }) {
  const state = STATE[row.state] ?? { label: row.state, tone: "muted" as Tone }
  const cell = { py: "2", pr: "4", borderBottomWidth: "1px", borderColor: "hairline" } as const
  return (
    <Box as="tr">
      <Box as="td" {...cell}>
        <Link href={initiativeHref(row.address)} color="tone.running.fg">
          {row.slug}
        </Link>
      </Box>
      <Box as="td" {...cell}>
        <Chip tone={state.tone} fontSize="12px">
          {state.label}
        </Chip>
      </Box>
      <Box as="td" {...cell}>
        {row.done} of {row.total} done
      </Box>
      <Box as="td" {...cell}>
        {currentText(row)}
      </Box>
    </Box>
  )
}

export function InitiativesPage({
  fetchList = fetchInitiatives,
}: {
  fetchList?: FetchInitiatives
}) {
  const conn = useConnectionContext()
  const { data, failed, source, retry } = useFetched("initiatives", fetchList, conn)
  const { active } = useActiveWorkspace(conn)
  // No active workspace means every project.
  const projects = (data?.projects ?? []).filter(
    (p) => p.initiatives.length > 0 && (!active || p.workspaces?.includes(active)),
  )
  useFetchedReady({ failed, source }, !!data && projects.length === 0)

  return (
    <DetailPage title={{ kind: "Initiatives" }}>
      <PageHeader title="Initiatives" icon={<PageIcon kind="initiative" />} />
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load the initiatives</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !data ? (
        <FullScreenLoader label="Loading initiatives" />
      ) : projects.length === 0 ? (
        <EmptyState title="No Initiatives yet. Start one with `gx initiatives new`." />
      ) : (
        projects.map((p) => (
          <Box key={p.project} role="region" aria-label={p.project} mt="6">
            <Text as="h2" fontFamily="condensed" fontSize="24px" fontWeight="600" mb="2">
              {p.project}
            </Text>
            <Box as="table" w="100%" borderCollapse="collapse">
              <Box as="thead">
                <Box as="tr">
                  <Box as="th" {...headCell}>
                    Initiative
                  </Box>
                  <Box as="th" {...headCell}>
                    State
                  </Box>
                  <Box as="th" {...headCell}>
                    Milestones
                  </Box>
                  <Box as="th" {...headCell}>
                    Current
                  </Box>
                </Box>
              </Box>
              <Box as="tbody">
                {p.initiatives.map((r) => (
                  <InitiativeTableRow key={r.address} row={r} />
                ))}
              </Box>
            </Box>
          </Box>
        ))
      )}
    </DetailPage>
  )
}
