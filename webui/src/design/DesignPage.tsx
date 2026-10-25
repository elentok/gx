import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { Navigate, useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { designHref } from "../hrefs";
import { useEpicDetail, type FetchEpicDetail } from "../epic/detail";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { DetailPage } from "../page/DetailPage";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import type { FetchChat } from "./chat";
import { useFocusReport, type PostFocus } from "./focus";
import { DesignChat } from "./ChatLog";
import { NewDesignForm, type NewDesignDraft } from "./NewDesignForm";
import type { OpenDesign, ResumeSession, StopSession } from "./open";
import { MobilePicker, Picker, useWide } from "./Picker";
import type { PostPrompt } from "./prompt";
import { ImplLink, Progress, showsImplLink } from "./Progress";
import type { FetchScreen } from "./screen";
import { SessionHeader, StartDesignButton } from "./SessionControls";
import type { SessionInfo } from "../generated/server";
import { designSession, liveSessions, IN_PROGRESS, pickerGroups } from "./view";

/** The injected fetchers and actions the Design page talks to the server through. */
export interface DesignDeps {
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  open?: OpenDesign;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
}

interface SessionStripProps {
  address: string;
  session?: SessionInfo;
  live: SessionInfo[];
  chart?: boolean;
  deps: DesignDeps;
  onSelect: (id: string) => void;
  onNewSession: () => void;
}

// The header above the chat: switch/stop controls, a way to start again, or the first Start button.
function SessionStrip({ address, session, live, chart, deps, onSelect, onNewSession }: SessionStripProps) {
  if (!session) {
    return (
      <Box display="flex" justifyContent="center" py="4">
        <StartDesignButton address={address} open={deps.open} />
      </Box>
    );
  }
  if (!chart) return <SessionHeader address={address} shown={session} live={live} onSelect={onSelect} open={deps.open} stop={deps.stop} />;
  if (session.status.state !== "exited") return null;
  return (
    <Box display="flex" justifyContent="flex-end">
      <Button size="sm" variant="outline" onClick={onNewSession}>
        New session
      </Button>
    </Box>
  );
}

// The Chat and Progress columns of one Design. Progress reads the Design's epic detail.
function DesignColumns({
  address,
  wide,
  deps,
  chart,
  onNewSession,
}: {
  address: string;
  wide: boolean;
  deps: DesignDeps;
  /** The Design has no map yet, so only its session can be started again, from the New Design form. */
  chart?: boolean;
  onNewSession: () => void;
}) {
  const conn = useConnectionContext();
  const { detail } = useEpicDetail(address, conn, deps.fetchDetail);
  const connState = useConnectionState(conn);
  const sessions = connState.snapshot?.sessions ?? [];
  const [selected, setSelected] = useState<string>();
  const done = detail?.steps?.some((s) => s.step === "done" && s.state === "done");
  const current = detail?.steps?.find((s) => s.state === "current");
  // Continue's subtitle: the next ticket while tickets resolve, else the step.
  const nextTitle = (current?.step === "resolve-tickets" && detail?.rows.find((r) => r.current)?.title) || current?.label;
  const live = liveSessions(address, sessions);
  // Switching shows an older live session; once it is gone the newest drives the page again.
  const session = live.find((s) => s.id === selected) ?? designSession(address, sessions);
  useFocusReport(conn, session?.id, connState.streams, deps.focus);
  return (
    <>
      <Box aria-label="Chat" role="region" minW="0" display="flex" flexDirection="column" gap="3">
        {wide && detail && showsImplLink(detail) && <ImplLink address={detail.impl_address!} />}
        <SessionStrip address={address} session={session} live={live} chart={chart} deps={deps} onSelect={setSelected} onNewSession={onNewSession} />
        <DesignChat
          session={session}
          fetchChat={deps.fetchChat}
          post={deps.post}
          done={done}
          step={current?.step}
          nextTitle={nextTitle}
          actions={{ onNewSession, resume: deps.resume, fetchScreen: deps.fetchScreen }}
        />
      </Box>
      {/* On a phone Progress sits right under the picker, above the chat. */}
      <Box aria-label="Progress" role="region" gridRow={{ base: 2, lg: "auto" }}>
        {detail && <Progress detail={detail} wide={wide} />}
      </Box>
    </>
  );
}

// `address` is the Design in the route; the bare route has none and opens the first In progress Design.
export function DesignPage({ address, fetchList, deps = {} }: { address?: string; fetchList?: FetchEpicList; deps?: DesignDeps }) {
  const conn = useConnectionContext();
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const { list, failed } = useEpicList(fetchList, conn);

  const navigate = useNavigate();
  const [draft, setDraft] = useState<NewDesignDraft>();

  const wide = useWide();
  const groups = list ? pickerGroups(list.epics, sessions) : [];
  const projectNames = [...new Set([...(list?.epics ?? []).map((e) => e.project), ...groups.flatMap((g) => g.rows.map((r) => r.row.project))])];
  const projects = new Set(list?.epics.filter((e) => e.kind === "map").map((e) => e.project)).size;
  // The form's project select starts on the Design on screen.
  const currentProject = address?.split(":")[0];
  const formProjects = currentProject ? [currentProject, ...projectNames.filter((p) => p !== currentProject)] : projectNames;
  const chartRow = groups.flatMap((g) => g.rows).find((r) => r.row.address === address)?.chart;
  const onNew = () => setDraft({});
  const onNewSession = () => {
    if (address) setDraft({ slug: address.slice(address.indexOf(":") + 1), project: currentProject });
  };

  const first = list && !address ? groups.find((g) => g.key === IN_PROGRESS)?.rows[0] : undefined;
  if (first) return <Navigate to={designHref(first.row.address).slice(1)} replace />;

  return (
    <DetailPage active="designs">
      {draft && (
        <NewDesignForm
          key={`${draft.slug}|${draft.project}`}
          projects={formProjects}
          initial={draft}
          open={deps.open}
          onStarted={(started) => {
            setDraft(undefined);
            navigate(designHref(started).slice(1));
          }}
          onCancel={() => setDraft(undefined)}
        />
      )}
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <Box display="grid" gridTemplateColumns={{ base: "1fr", lg: "260px minmax(0, 1fr) 300px" }} gap="4" minH="60vh">
          {wide ? (
            <Picker groups={groups} projects={projects} current={address} onNew={onNew} />
          ) : (
            <MobilePicker groups={groups} projects={projects} current={address} onNew={onNew} />
          )}
          {address ? (
            <DesignColumns address={address} wide={wide} deps={deps} chart={chartRow} onNewSession={onNewSession} />
          ) : (
            <>
              <Box aria-label="Chat" role="region">
                <EmptyState title="No Design in progress.">
                  <Button tone="running" onClick={onNew}>
                    New Design
                  </Button>
                </EmptyState>
              </Box>
              <Box aria-label="Progress" role="region" />
            </>
          )}
        </Box>
      )}
    </DetailPage>
  );
}
