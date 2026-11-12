import { Box, Spinner as ChakraSpinner, Text, chakra } from "@chakra-ui/react"
import { useState } from "react"
import { useConnectionContext, useConnectionState } from "../connection"
import { flowSession, liveFlowSessions } from "../design/view"
import { stateToken, type OpenFlow, type ResumeSession, type StopSession } from "../design/open"
import {
  postAbandonTriage,
  postOpenTriage,
  type AbandonTriage,
  type ApproveTriage,
  type SendBackTriage,
  type SaveTriageRow,
  type TriageRowAction,
} from "./api"
import { useDesignWrite } from "../design/SessionControls"
import type { FetchChat } from "../design/chat"
import type { PostFocus } from "../design/focus"
import type { PostAnswer, PostPrompt } from "../design/prompt"
import type { FetchScreen } from "../design/screen"
import {
  TriageAbandoned,
  TriageDone,
  TriageNone,
  type SessionInfo,
  type TriageView,
} from "../generated/server"
import { Modal } from "../ui/modal/Modal"
import { TriageHistory } from "./History"
import { ApproveShell, approveSubRows, useApprove, useApproveView } from "./ApproveShell"
import { DetailPage } from "../page/DetailPage"
import { PageIcon } from "../page/PageIcon"
import { Button } from "../ui/button/Button"
import { ProgressRing } from "../ui/progress-ring/ProgressRing"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { triageModel } from "../stepped/model"
import { SteppedPage } from "../stepped/SteppedPage"
import { useWide } from "../useWide"
import { DoneQueue, DoneView } from "./DoneView"
import type { FetchEpicList } from "../epics/list"
import type { PostEpicWrite } from "../epics/queue"
import { QuestionsView } from "./QuestionsView"
import { ReviewView } from "./ReviewView"
import { WriteView, rowsWritten } from "./WriteView"
import { useTriage, type FetchTriage } from "./useTriage"

/** The injected fetchers and actions the Triage page talks to the server through. */
export interface TriageDeps {
  fetchTriage?: FetchTriage
  fetchChat?: FetchChat
  post?: PostPrompt
  answer?: PostAnswer
  open?: OpenFlow
  stop?: StopSession
  resume?: ResumeSession
  fetchScreen?: FetchScreen
  focus?: PostFocus
  abandon?: AbandonTriage
  approve?: ApproveTriage
  sendBack?: SendBackTriage
  saveRow?: SaveTriageRow
  retry?: TriageRowAction
  keep?: TriageRowAction
  fetchEpics?: FetchEpicList
  queue?: PostEpicWrite
}

const FLOW = "triage"
const APPROVE = "approve"
const WRITE = "write"

/**
 * Start, or Continue once a session exists. The open route returns a live
 * session as it is, so Continue on an idle one asks for a fresh session
 * (`new`, with the idle session's state token): that is the /clear and skill
 * the run-end line asks for. A working, blocked or rate-limited session is left alone.
 */
function HeaderButton({
  project,
  session,
  open = postOpenTriage,
}: {
  project: string
  session?: SessionInfo
  open?: OpenFlow
}) {
  const { run, pending } = useDesignWrite()
  const state = session?.status.state
  const busy = !!session && state !== "exited" && state !== "idle"
  const body = session && state === "idle" ? { new: true, state: stateToken(session) } : {}
  return (
    <Button
      tone="running"
      disabled={pending || busy}
      onClick={() => run(() => open({ epic: project, ...body }))}
    >
      {pending && <ChakraSpinner size="sm" />}
      {session ? "Continue" : "Start"}
    </Button>
  )
}

type Phase = "approve" | "write" | "done" | "working"

function phaseOf(view: TriageView): Phase {
  if (view.state === TriageDone) return "done"
  const current = view.steps.find((s) => s.state === "current")?.step
  return current === APPROVE ? "approve" : current === WRITE ? "write" : "working"
}

// The verdicts the Triage has saved so far, out of its rows.
const verdictsSaved = (view: TriageView) =>
  `${view.rows.filter((r) => r.verdict).length} of ${view.rows.length}`

/** Shown above the chat while the Triage's session runs. */
function RunningBanner({ phase, view }: { phase: Phase; view: TriageView }) {
  return (
    <Box
      display="flex"
      alignItems="center"
      gap="2"
      px="3"
      py="2"
      borderWidth="1px"
      borderColor="tone.running.fg"
      borderRadius="md"
      fontSize="sm"
    >
      <Box aria-hidden w="8px" h="8px" borderRadius="full" bg="tone.running.fg" />
      {phase === "write"
        ? `Write session running · rows written ${rowsWritten(view)} of ${view.rows.length}`
        : `Triage session running · verdicts saved ${verdictsSaved(view)}`}
    </Box>
  )
}

/** Confirms Abandon. It writes nothing to tickets, so rows already written stay written. */
function AbandonDialog({
  project,
  revision,
  abandon = postAbandonTriage,
  onClose,
}: {
  project: string
  revision?: number
  abandon?: AbandonTriage
  onClose: () => void
}) {
  const { run, pending } = useDesignWrite()
  return (
    <Modal label="Abandon Triage" onClose={onClose} p="4" gap="3">
      <Text fontWeight="semibold">Abandon this Triage?</Text>
      <Text>It moves to the past Triages. Rows already written stay written.</Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="ghost" disabled={pending} onClick={onClose}>
          Cancel
        </Button>
        <Button
          tone="paused"
          disabled={pending}
          onClick={() =>
            run(() => abandon({ project, revision: revision ?? 0 }), { onDone: onClose })
          }
        >
          {pending && <ChakraSpinner size="sm" />}
          Abandon
        </Button>
      </Box>
    </Modal>
  )
}

function TriageBody({
  project,
  view,
  deps,
  refetch,
}: {
  project: string
  view: TriageView
  deps: TriageDeps
  refetch: () => void
}) {
  const [abandoning, setAbandoning] = useState(false)
  const approveView = useApproveView(view.revision, view.open_questions)
  const approval = useApprove({
    project,
    view,
    refetch,
    approve: deps.approve,
    carryReview: approveView.carryReview,
  })
  const phase = phaseOf(view)
  const atApprove = phase === "approve"
  const atWrite = phase === "write"
  const atDone = phase === "done"
  const { fetchChat, post, answer, open, stop, resume, fetchScreen, focus } = deps
  const steppedActions = { fetchChat, post, answer, open, stop, resume, fetchScreen, focus }
  const wide = useWide()
  const conn = useConnectionContext()
  const sessions = useConnectionState(conn).snapshot?.sessions ?? []
  const live = liveFlowSessions(FLOW, project, sessions)
  const model = triageModel({ key: project, steps: view.steps, ringPercent: view.ring_percent })
  return (
    <>
      {abandoning && (
        <AbandonDialog
          project={project}
          revision={view.revision}
          abandon={deps.abandon}
          onClose={() => setAbandoning(false)}
        />
      )}
      <SteppedPage
        menuItems={
          view.can_abandon
            ? [
                {
                  value: "abandon-triage",
                  label: "Abandon Triage",
                  danger: true,
                  onSelect: () => setAbandoning(true),
                },
              ]
            : undefined
        }
        footer={<TriageHistory items={view.history} />}
        address={project}
        title={`Triage: ${project}`}
        icon={<PageIcon kind="triage" />}
        wide={wide}
        model={model}
        ring={(size) => (
          <ProgressRing
            progress={view.ring_percent / 100}
            tone="running"
            kind="followups"
            label={size === "compact" ? "" : `${view.ring_percent}%`}
            size={size}
            live={live.length > 0}
          />
        )}
        live={live}
        current={flowSession(FLOW, project, sessions)}
        actions={steppedActions}
        startLabel="Start"
        beforeRing={
          atDone && view.result?.ready_epic ? (
            <DoneQueue
              epic={view.result.ready_epic}
              fetchEpics={deps.fetchEpics}
              queue={deps.queue}
            />
          ) : undefined
        }
        headerButton={(session) =>
          atDone ? null : atApprove ? (
            <Button
              tone="done"
              disabled={!!approval.blocked || approval.pending}
              onClick={approval.approve}
            >
              {approval.pending && <ChakraSpinner size="sm" />}
              Approve
            </Button>
          ) : (
            <HeaderButton project={project} session={session} open={deps.open} />
          )
        }
        noContinue={atApprove}
        stepRows={atApprove ? { [APPROVE]: approveSubRows(view, approveView.view) } : undefined}
        working={() => ({ step: model.current?.label })}
        subtitleSuffix={
          atApprove
            ? ` · ${approveView.view === "questions" ? `Questions · ${view.open_questions} left` : `Review · ${view.rows.length} rows`}`
            : view.step_note && ` · ${view.step_note}`
        }
        metaLink={
          <chakra.a href={`#/e/${project}:follow-ups`} color="text.dim" textDecoration="underline">
            Triage of {project}:follow-ups ↗
          </chakra.a>
        }
        chatTop={() => (
          <>
            {atApprove && (
              <ApproveShell
                view={view}
                shown={approveView.view}
                onShow={approveView.setView}
                approval={approval}
                questions={
                  <QuestionsView
                    project={project}
                    view={view}
                    refetch={refetch}
                    save={deps.saveRow}
                    sendBack={deps.sendBack}
                  />
                }
                review={
                  <ReviewView project={project} view={view} refetch={refetch} save={deps.saveRow} />
                }
              />
            )}
            {atDone && view.result && <DoneView result={view.result} />}
            {atWrite && (
              <WriteView
                project={project}
                view={view}
                refetch={refetch}
                retry={deps.retry}
                keep={deps.keep}
              />
            )}
            {live.some((s) => s.status.state !== "idle") && <RunningBanner phase={phase} view={view} />}
          </>
        )}
      />
    </>
  )
}

export function TriagePage({ project, deps = {} }: { project: string; deps?: TriageDeps }) {
  return (
    <DetailPage fill title={{ kind: "Triage", name: project }}>
      <TriagePageBody project={project} deps={deps} />
    </DetailPage>
  )
}

// The body reads `useWide`, so it renders inside DetailPage's page mode.
function TriagePageBody({ project, deps }: { project: string; deps: TriageDeps }) {
  const conn = useConnectionContext()
  const { detail, notFound, refetch } = useTriage(project, conn, deps.fetchTriage)
  // An abandoned latest record is no Triage to work on; it stays in history.
  if (notFound || detail?.state === TriageNone || detail?.state === TriageAbandoned)
    return (
      <Box display="flex" flexDirection="column" gap="4">
        <Text color="text.muted">No Triage for {project}</Text>
        {detail && <TriageHistory items={detail.history} />}
      </Box>
    )
  if (!detail) return <FullScreenLoader label="Loading Triage" />
  return <TriageBody project={project} view={detail} deps={deps} refetch={refetch} />
}
