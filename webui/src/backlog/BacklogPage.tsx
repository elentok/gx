import { Box, chakra, Spinner, Text } from "@chakra-ui/react"
import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import type { BacklogEpic } from "../generated/server"
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
  backlog,
  start,
  onClose,
}: {
  project: string
  backlog: BacklogEpic
  start: PostStartDesign
  onClose: () => void
}) {
  const navigate = useNavigate()
  const [hint, setHint] = useState("")
  // The refusal stays in the dialog, so no toast.
  const { run, pending, refusal, error } = useWrite({ toastRefusal: false })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void run(() => start({ project, backlog: backlog.name, hint: hint.trim() || undefined }), {
      onDone: (res) => res.session && navigate(agentPath(res.session.id)),
    })
  }
  const problem = refusal ? refusalText(refusal) : error
  return (
    <BottomSheet label={`Start Design for ${backlog.name}`} onClose={onClose}>
      <Box as="form" onSubmit={submit} display="flex" flexDirection="column" gap="3" p="2">
        <Text as="h2" fontFamily="condensed" fontSize="xl" fontWeight="600">
          Start Design for {backlog.name}
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

function BacklogRow({
  project,
  backlog,
  onStart,
  finish,
  onChanged,
}: {
  project: string
  backlog: BacklogEpic
  onStart: () => void
  finish: PostFinishMove
  onChanged: () => void
}) {
  const { run, pending } = useWrite()
  const move = backlog.unfinished
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
        {backlog.name}
      </Text>
      <Text color="text.muted" flexShrink="0">
        {backlog.open} open
      </Text>
      {move && (
        <Button
          emphasis="secondary"
          flexShrink="0"
          disabled={pending}
          onClick={() =>
            // The list names the map by address; the route takes its bare name.
            void run(
              () =>
                finish({ project, backlog: backlog.name, map: move.map.slice(project.length + 1) }),
              { onDone: onChanged },
            )
          }
        >
          Finish move
        </Button>
      )}
      <Button emphasis="main" flexShrink="0" disabled={backlog.open === 0} onClick={onStart}>
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
  const [starting, setStarting] = useState<{ project: string; backlog: BacklogEpic }>()
  // The sidebar workspace filter only affects the sidebar, so list every project.
  const projects = (data?.projects ?? []).filter((p) => p.epics.length > 0)
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
        <EmptyState title="No epics yet." />
      ) : (
        projects.map((p) => (
          <Box key={p.project} role="region" aria-label={p.project} mt="6">
            <Text as="h2" fontFamily="condensed" fontSize="24px" fontWeight="600" mb="2">
              {p.project}
            </Text>
            <Box as="ul" listStyleType="none" m="0" p="0">
              {p.epics.map((a) => (
                <BacklogRow
                  key={a.address}
                  project={p.project}
                  backlog={a}
                  finish={finish}
                  onChanged={retry}
                  onStart={() => setStarting({ project: p.project, backlog: a })}
                />
              ))}
            </Box>
          </Box>
        ))
      )}
      {starting && (
        <StartDialog
          project={starting.project}
          backlog={starting.backlog}
          start={start}
          onClose={() => setStarting(undefined)}
        />
      )}
    </DetailPage>
  )
}
