import { Box, Spinner as ChakraSpinner, Text, chakra } from "@chakra-ui/react";
import { useConnectionContext, useConnectionState } from "../connection";
import { flowSession, liveFlowSessions } from "../design/view";
import { postOpenTriage, type OpenFlow, type ResumeSession, type StopSession } from "../design/open";
import { useDesignWrite } from "../design/SessionControls";
import type { FetchChat } from "../design/chat";
import type { PostFocus } from "../design/focus";
import type { PostPrompt } from "../design/prompt";
import type { FetchScreen } from "../design/screen";
import { TriageNone, type SessionInfo, type TriageView } from "../generated/server";
import { DetailPage } from "../page/DetailPage";
import { PageIcon } from "../page/PageIcon";
import { Button } from "../ui/button/Button";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { Spinner } from "../ui/spinner/Spinner";
import { triageModel } from "../stepped/model";
import { SteppedPage } from "../stepped/SteppedPage";
import { useWide } from "../useWide";
import { useTriage, type FetchTriage } from "./useTriage";

/** The injected fetchers and actions the Triage page talks to the server through. */
export interface TriageDeps {
  fetchTriage?: FetchTriage;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  open?: OpenFlow;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
}

const FLOW = "triage";

/** Start, or Continue once a session exists. The open route returns the live Triage, so it never starts a second one. */
function HeaderButton({ project, session, open = postOpenTriage }: { project: string; session?: SessionInfo; open?: OpenFlow }) {
  const { run, pending } = useDesignWrite();
  const running = !!session && session.status.state !== "exited";
  return (
    <Button tone="running" disabled={pending || running} onClick={() => run(() => open({ epic: project }))}>
      {pending && <ChakraSpinner size="sm" />}
      {session ? "Continue" : "Start"}
    </Button>
  );
}

// The verdicts the Triage has saved so far, out of its rows.
const verdictsSaved = (view: TriageView) => `${view.rows.filter((r) => r.verdict).length} of ${view.rows.length}`;

function TriageBody({ project, view, deps }: { project: string; view: TriageView; deps: TriageDeps }) {
  const wide = useWide();
  const conn = useConnectionContext();
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const live = liveFlowSessions(FLOW, project, sessions);
  const model = triageModel({ key: project, steps: view.steps, ringPercent: view.ring_percent });
  return (
    <SteppedPage
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
      actions={deps}
      startLabel="Start"
      headerButton={(session) => <HeaderButton project={project} session={session} open={deps.open} />}
      working={() => ({ step: model.current?.label })}
      subtitleSuffix={view.step_note && ` · ${view.step_note}`}
      metaLink={
        <chakra.a href={`#/e/${project}:follow-ups`} color="text.dim" textDecoration="underline">
          Triage of {project}:follow-ups ↗
        </chakra.a>
      }
      chatTop={() =>
        live.length > 0 && (
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
            Triage session running · verdicts saved {verdictsSaved(view)}
          </Box>
        )
      }
    />
  );
}

export function TriagePage({ project, deps = {} }: { project: string; deps?: TriageDeps }) {
  return (
    <DetailPage fill title={{ kind: "Triage", name: project }}>
      <TriagePageBody project={project} deps={deps} />
    </DetailPage>
  );
}

// The body reads `useWide`, so it renders inside DetailPage's page mode.
function TriagePageBody({ project, deps }: { project: string; deps: TriageDeps }) {
  const conn = useConnectionContext();
  const { detail, notFound } = useTriage(project, conn, deps.fetchTriage);
  if (notFound || detail?.state === TriageNone) return <Text color="text.muted">No Triage for {project}</Text>;
  if (!detail) return <Spinner label="Loading Triage" />;
  return <TriageBody project={project} view={detail} deps={deps} />;
}
