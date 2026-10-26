import { Box, Text } from "@chakra-ui/react";
import { useCallback, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { designHref } from "../hrefs";
import { useEpicDetail, type FetchEpicDetail } from "../epic/detail";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { BackLink } from "../nav/BackLink";
import { JumpMenu } from "../nav/JumpMenu";
import { readPageCache, writePageCache } from "../pageCache";
import { DetailPage } from "../page/DetailPage";
import { Button } from "../ui/button/Button";
import { Spinner } from "../ui/spinner/Spinner";
import type { FetchChat } from "./chat";
import { useFocusReport, type PostFocus } from "./focus";
import { DesignChat } from "./ChatLog";
import { NewDesignForm, type NewDesignDraft } from "./NewDesignForm";
import type { OpenDesign, ResumeSession, StopSession } from "./open";
import type { PostPrompt } from "./prompt";
import { ImplLink, Progress, showsImplLink } from "./Progress";
import type { FetchScreen } from "./screen";
import { SessionHeader, StartDesignButton } from "./SessionControls";
import type { SessionInfo } from "../generated/server";
import { useWide } from "./useWide";
import { designSession, liveSessions } from "./view";

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
function SessionStrip({
  address,
  session,
  live,
  chart,
  deps,
  onSelect,
  onNewSession,
}: SessionStripProps) {
  if (!session) {
    return <StartDesignButton address={address} open={deps.open} />;
  }
  if (!chart)
    return (
      <SessionHeader
        address={address}
        shown={session}
        live={live}
        onSelect={onSelect}
        open={deps.open}
        stop={deps.stop}
      />
    );
  if (session.status.state !== "exited") return null;
  return (
    <Box display="flex" justifyContent="flex-end">
      <Button size="sm" emphasis="secondary" onClick={onNewSession}>
        New session
      </Button>
    </Box>
  );
}

// The title row and the Chat and Progress columns of one Design. Progress reads the Design's epic detail.
function DesignBody({
  address,
  wide,
  deps,
  chart,
  fetchList,
  onNewSession,
}: {
  address: string;
  wide: boolean;
  deps: DesignDeps;
  fetchList?: FetchEpicList;
  /** The Design has no map yet, so only its session can be started again, from the New Design form. */
  chart?: boolean;
  onNewSession: () => void;
}) {
  const conn = useConnectionContext();
  const { detail } = useEpicDetail(address, conn, deps.fetchDetail);
  const connState = useConnectionState(conn);
  const sessions = connState.snapshot?.sessions ?? [];
  const [selected, setSelected] = useState<string>();
  const done = detail?.steps?.some(
    (s) => s.step === "done" && s.state === "done",
  );
  const current = detail?.steps?.find((s) => s.state === "current");
  // Continue's subtitle: the next ticket while tickets resolve, else the step.
  const nextTitle =
    (current?.step === "resolve-tickets" &&
      detail?.rows.find((r) => r.current)?.title) ||
    current?.label;
  const live = liveSessions(address, sessions);
  // Switching shows an older live session; once it is gone the newest drives the page again.
  const session =
    live.find((s) => s.id === selected) ?? designSession(address, sessions);
  // The busy row follows what the agent reported; Progress and Continue keep following the map.
  const running =
    detail?.steps?.find((s) => s.step === session?.reported_step) ?? current;
  // Without the map detail there is no step to name, so the row reads a bare "Working".
  const working = {
    step: running?.label,
    title:
      running?.step === "resolve-tickets"
        ? session?.reported_title ||
          detail?.rows.find((r) => r.current)?.title
        : undefined,
  };
  useFocusReport(conn, session?.id, connState.streams, deps.focus);
  return (
    <>
      <Box
        mt="4"
        mb="3"
        display="flex"
        alignItems="flex-end"
        justifyContent="space-between"
        gap="4"
        flex="none"
      >
        <Box minW="0">
          <BackLink to="/epics" label="← Epics" />
          <JumpMenu name={address.slice(address.indexOf(":") + 1)} current={address} fetchList={fetchList} />
        </Box>
        <SessionStrip
          address={address}
          session={session}
          live={live}
          chart={chart}
          deps={deps}
          onSelect={setSelected}
          onNewSession={onNewSession}
        />
      </Box>
      <Box
        display="grid"
        gridTemplateColumns={{ base: "1fr", lg: "minmax(0, 1fr) 300px" }}
        gridTemplateRows={{ base: "auto minmax(0, 1fr)", lg: "minmax(0, 1fr)" }}
        gap="4"
        flex="1"
        minH="0"
      >
        <Box
          aria-label="Chat"
          role="region"
          minW="0"
          minH="0"
          display="flex"
          flexDirection="column"
          gap="3"
          gridRow={{ base: 2, lg: 1 }}
          gridColumn={1}
        >
          {wide && detail && showsImplLink(detail) && (
            <ImplLink address={detail.impl_address!} />
          )}
          <DesignChat
            session={session}
            fetchChat={deps.fetchChat}
            post={deps.post}
            done={done}
            nextTitle={nextTitle}
            working={working}
            actions={{
              onNewSession,
              resume: deps.resume,
              fetchScreen: deps.fetchScreen,
            }}
          />
        </Box>
        {/* On a phone Progress sits above the chat. */}
        <Box
          aria-label="Progress"
          role="region"
          gridRow={1}
          gridColumn={{ base: 1, lg: 2 }}
          minH="0"
          overflowY="auto"
        >
          {detail && <Progress detail={detail} wide={wide} />}
        </Box>
      </Box>
    </>
  );
}

const NEW_DESIGN_CACHE = "design.new";

export function DesignPage({
  address,
  fetchList,
  deps = {},
}: {
  address: string;
  fetchList?: FetchEpicList;
  deps?: DesignDeps;
}) {
  const conn = useConnectionContext();
  const { list, failed } = useEpicList(fetchList, conn);

  const navigate = useNavigate();
  // The open form and its text survive a reload, like a half-answered round (see useRoundDraft).
  const [draft, setDraftState] = useState<NewDesignDraft | undefined>(
    () => readPageCache<NewDesignDraft>(NEW_DESIGN_CACHE) ?? undefined,
  );
  const setDraft = (d: NewDesignDraft | undefined) => {
    writePageCache(NEW_DESIGN_CACHE, d ?? null);
    setDraftState(d);
  };
  // Saves without touching state: the form's key comes from the draft, so a state change would remount it.
  const saveDraft = useCallback(
    (d: NewDesignDraft) => writePageCache(NEW_DESIGN_CACHE, d),
    [],
  );

  const wide = useWide();
  const projectNames = [...new Set((list?.epics ?? []).map((e) => e.project))];
  // The form's project select starts on the Design on screen.
  const currentProject = address.split(":")[0];
  const formProjects = [
    currentProject,
    ...projectNames.filter((p) => p !== currentProject),
  ];
  // No map yet: Chart has not made the Design's epic.
  const chart = !list?.epics.some((e) => e.address === address);
  const onNewSession = () =>
    setDraft({
      slug: address.slice(address.indexOf(":") + 1),
      project: currentProject,
    });

  return (
    <DetailPage active="epics" fill>
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
          onChange={saveDraft}
        />
      )}
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <DesignBody
          address={address}
          wide={wide}
          deps={deps}
          chart={chart}
          fetchList={fetchList}
          onNewSession={onNewSession}
        />
      )}
    </DetailPage>
  );
}
