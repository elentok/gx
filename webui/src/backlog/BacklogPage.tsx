import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import type { BacklogArea } from "../generated/server"
import { agentPath } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { Button } from "../ui/button/Button"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { BottomSheet } from "../ui/modal/BottomSheet"
import { useFetched } from "../useFetched"
import { useFetchedReady } from "../useReady"
import { refusalText, useWrite } from "../useWrite"
import { useActiveWorkspace } from "../workspaces/useActiveWorkspace"
import {
  fetchBacklog,
  postFinishMove,
  postStartDesign,
  type FetchBacklog,
  type PostFinishMove,
  type PostStartDesign,
} from "./backlog"

function StartDialog({
  project,
  area,
  start,
  onClose,
}: {
  project: string
  area: BacklogArea
  start: PostStartDesign
  onClose: () => void
}) {
  const navigate = useNavigate()
  const [hint, setHint] = useState("")
  // The refusal stays in the dialog, so no toast.
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void run(() => start({ project, area: area.name, hint: hint.trim() || undefined }), {
      onDone: (res) => res.session && navigate(agentPath(res.session.id)),
    })
  }
  const problem = refusal ? refusalText(refusal) : error
  return (
    <BottomSheet label={`Start Design for ${area.name}`} onClose={onClose}>
      <Box as="form" onSubmit={submit} display="flex" flexDirection="column" gap="3" p="2">
        <Text as="h2" fontFamily="condensed" fontSize="xl" fontWeight="600">
          Start Design for {area.name}
        </Text>
        <chakra.textarea
          aria-label="Hint"
          placeholder="Optional: what should the theme be about?"
          value={hint}
          onChange={(e) => setHint(e.target.value)}
          rows={3}
          fontSize="16px"
          bg="surface.raised"
          borderWidth="1px"
          borderColor="hairline"
          borderRadius="8px"
          px="3"
          py="2"
        />
        {problem && (
          <Text role="alert" fontSize="sm" color="tone.waiting.fg">
            {problem}
          </Text>
        )}
        <Box display="flex" justifyContent="flex-end" gap="2">
          <Button type="button" emphasis="ghost" tone="muted" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" tone="running" disabled={pending}>
            {pending && <Spinner size="sm" />}
            Start
          </Button>
        </Box>
      </Box>
    </BottomSheet>
  )
}

function AreaRow({
  project,
  area,
  onStart,
  finish,
  onChanged,
}: {
  project: string
  area: BacklogArea
  onStart: () => void
  finish: PostFinishMove
  onChanged: () => void
}) {
  const { run, pending } = useWrite()
  const move = area.unfinished
  return (
    <Box
      as="li"
      display="flex"
      alignItems="center"
      gap="3"
      py="2"
      borderBottomWidth="1px"
      borderColor="hairline"
    >
      <Text flex="1" minW="0" truncate fontWeight="600">
        {area.name}
      </Text>
      <Text color="text.muted" flexShrink="0">
        {area.open} open
      </Text>
      {move && (
        <Button
          emphasis="secondary"
          flexShrink="0"
          disabled={pending}
          onClick={() =>
            // The list names the map by address; the route takes its bare name.
            void run(
              () => finish({ project, area: area.name, map: move.map.slice(project.length + 1) }),
              { onDone: onChanged },
            )
          }
        >
          Finish move
        </Button>
      )}
      <Button emphasis="main" flexShrink="0" disabled={area.open === 0} onClick={onStart}>
        Start Design
      </Button>
    </Box>
  )
}

export function BacklogPage({
  fetchList = fetchBacklog,
  start = postStartDesign,
  finish = postFinishMove,
}: {
  fetchList?: FetchBacklog
  start?: PostStartDesign
  finish?: PostFinishMove
}) {
  const conn = useConnectionContext()
  const { data, failed, source, retry } = useFetched("backlog", fetchList, conn)
  const { active } = useActiveWorkspace(conn)
  const [starting, setStarting] = useState<{ project: string; area: BacklogArea }>()
  // No active workspace means every project.
  const projects = (data?.projects ?? []).filter(
    (p) => p.areas.length > 0 && (!active || p.workspaces?.includes(active)),
  )
  useFetchedReady({ failed, source }, !!data && projects.length === 0)

  return (
    <DetailPage title={{ kind: "Backlog" }}>
      <PageHeader title="Backlog" icon={<PageIcon kind="backlog" />} />
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load the backlog</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !data ? (
        <FullScreenLoader label="Loading backlog" />
      ) : projects.length === 0 ? (
        <EmptyState title="No areas yet." />
      ) : (
        projects.map((p) => (
          <Box key={p.project} role="region" aria-label={p.project} mt="6">
            <Text as="h2" fontFamily="condensed" fontSize="24px" fontWeight="600" mb="2">
              {p.project}
            </Text>
            <Box as="ul" listStyleType="none" m="0" p="0">
              {p.areas.map((a) => (
                <AreaRow
                  key={a.address}
                  project={p.project}
                  area={a}
                  finish={finish}
                  onChanged={retry}
                  onStart={() => setStarting({ project: p.project, area: a })}
                />
              ))}
            </Box>
          </Box>
        ))
      )}
      {starting && (
        <StartDialog
          project={starting.project}
          area={starting.area}
          start={start}
          onClose={() => setStarting(undefined)}
        />
      )}
    </DetailPage>
  )
}
