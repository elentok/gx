import { Box, Text, chakra } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicDetail, type FetchEpicDetail } from "../epic/detail";
import { EpicRing } from "../epic/EpicRing";
import { PageHeader } from "../page/PageHeader";
import { useEpicList, waitingCount, type EpicList, type FetchEpicList } from "../epics/list";
import { JumpMenu } from "../nav/JumpMenu";
import { DetailPage } from "../page/DetailPage";
import { Spinner } from "../ui/spinner/Spinner";
import { roundKeyOf, useSessionChat, type FetchChat } from "./chat";
import { useFocusReport, type PostFocus } from "./focus";
import { DesignChat } from "./ChatLog";
import type { DeleteDesign, OpenDesign, ResumeSession, StopSession } from "./open";
import type { PostPrompt } from "./prompt";
import { ImplLink, Progress, StepLine, StepsSheet, showsImplLink } from "./Progress";
import type { FetchScreen } from "./screen";
import { DesignActionsMenu, Failure, StartDesignButton } from "./SessionControls";
import type { EpicListRow, SessionInfo } from "../generated/server";
import { useWide } from "../useWide";
import { designSession, liveSessions } from "./view";

/** The injected fetchers and actions the Design page talks to the server through. */
export interface DesignDeps {
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  open?: OpenDesign;
  remove?: DeleteDesign;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
}

interface SessionStripProps {
  address: string;
  session?: SessionInfo;
  live: SessionInfo[];
  deps: DesignDeps;
  onSelect: (id: string) => void;
  startFailure?: string;
  /** The map has no Destination and no tickets yet, so it can still be deleted. */
  deletable?: boolean;
}

// The header above the chat: switch/stop controls, a way to start again, or the first Start button.
function SessionStrip({
  address,
  session,
  live,
  deps,
  onSelect,
  startFailure,
  deletable,
}: SessionStripProps) {
  const navigate = useNavigate();
  // The reason is for the failed start only: a live session means a retry worked.
  const reason = !session || session.status.state === "exited" ? startFailure : undefined;
  return (
    <Box display="flex" flexDirection="column" alignItems="flex-end" gap="1">
      <Failure>{reason ?? ""}</Failure>
      {!session && <StartDesignButton address={address} open={deps.open} />}
      <DesignActionsMenu
        address={address}
        shown={session}
        live={live}
        deletable={deletable}
        onSelect={onSelect}
        onDeleted={() => navigate("/epics")}
        open={deps.open}
        stop={deps.stop}
        remove={deps.remove}
      />
    </Box>
  );
}

// The title row and the Chat and Progress columns of one Design. Progress reads the Design's epic detail.
function DesignBody({
  address,
  wide,
  deps,
  epics,
  startFailure,
}: {
  address: string;
  wide: boolean;
  deps: DesignDeps;
  epics: EpicListRow[];
  startFailure?: string;
}) {
  const conn = useConnectionContext();
  const { detail, refetch } = useEpicDetail(address, conn, deps.fetchDetail);
  const connState = useConnectionState(conn);
  const sessions = connState.snapshot?.sessions ?? [];
  const [selected, setSelected] = useState<string>();
  const [toggle, setToggle] = useState<{ key: string; on: boolean }>();
  const [stepsOpen, setStepsOpen] = useState(false);
  const done = detail?.steps?.some(
    (s) => s.step === "done" && s.state === "done",
  );
  const current = detail?.steps?.find((s) => s.state === "current");
  // Continue's subtitle: the next ticket while tickets resolve, else the step.
  const nextTitle =
    (current?.step === "resolve-tickets" &&
      detail?.rows.find((r) => r.current)?.title) ||
    current?.label;
  const name = address.slice(address.indexOf(":") + 1);
  // The card speaks for this page only while its active (first) address is this Design.
  const card = epics.find((e) => e.addresses?.[0] === address);
  const live = liveSessions(address, sessions);
  // Switching shows an older live session; once it is gone the newest drives the page again.
  const session =
    live.find((s) => s.id === selected) ?? designSession(address, sessions);
  const claude = session?.harness === "claude";
  const { detail: chat, refetch: refetchChat } = useSessionChat(session && claude ? session.id : undefined, conn, deps.fetchChat);
  // Derived, not stored, so it cannot go stale: a round opens in Questions, and the reader's toggle holds for
  // that session and round only. The message list and the impl link both read it.
  const viewKey = `${session?.id}|${roundKeyOf(chat)}`;
  const questions = claude && (chat?.round?.length ?? 0) > 0 && (toggle?.key === viewKey ? toggle.on : true);
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
  // `deletable` depends on the live sessions, and a session stop is no ticket event.
  const liveKey = live.map((s) => s.id).join(",");
  const seenLiveKey = useRef(liveKey);
  useEffect(() => {
    if (seenLiveKey.current === liveKey) return;
    seenLiveKey.current = liveKey;
    refetch();
  }, [liveKey, refetch]);
  const ring = (size: "md" | "sm" | "compact") => <EpicRing detail={detail!} card={card} sessions={sessions} size={size} />;
  return (
    <>
      {stepsOpen && detail && <StepsSheet detail={detail} ring={ring("compact")} onClose={() => setStepsOpen(false)} />}
      <Box
        mt="4"
        mb="3"
        display="flex"
        alignItems="flex-start"
        justifyContent="space-between"
        gap="4"
        flex="none"
      >
        <Box minW="0">
          <PageHeader
            title={name}
            lead={
              detail &&
              (wide ? (
                ring("md")
              ) : (
                <chakra.button type="button" aria-label="Show progress steps" aria-haspopup="dialog" cursor="pointer" onClick={() => setStepsOpen(true)}>
                  {ring("sm")}
                </chakra.button>
              ))
            }
            heading={
              <>
                <JumpMenu name={name} current={address} epics={epics} />
                {!wide && detail && <StepLine detail={detail} onOpen={() => setStepsOpen(true)} />}
              </>
            }
          />
        </Box>
        <SessionStrip
          address={address}
          session={session}
          live={live}
          deps={deps}
          onSelect={setSelected}
          startFailure={startFailure}
          deletable={detail?.deletable}
        />
      </Box>
      <Box
        display="grid"
        gridTemplateColumns={{ base: "1fr", lg: "minmax(0, 1fr) 300px" }}
        gridTemplateRows={"minmax(0, 1fr)"}
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
          gridRow={1}
          gridColumn={1}
        >
          {!questions && detail && showsImplLink(detail) && (
            <ImplLink address={detail.impl_address!} />
          )}
          <DesignChat
            session={session}
            chat={chat}
            refetch={refetchChat}
            post={deps.post}
            done={done}
            nextTitle={nextTitle}
            working={working}
            questions={questions}
            onQuestions={(on) => setToggle({ key: viewKey, on })}
            actions={{
              address,
              open: deps.open,
              resume: deps.resume,
              fetchScreen: deps.fetchScreen,
            }}
          />
        </Box>
        {wide && (
          <Box aria-label="Progress" role="region" gridRow={1} gridColumn={2} minH="0" overflowY="auto">
            {detail && <Progress detail={detail} />}
          </Box>
        )}
      </Box>
    </>
  );
}

type DesignPageProps = {
  address: string;
  fetchList?: FetchEpicList;
  deps?: DesignDeps;
  /** Why the session did not start, when the New Design modal created the map but the start failed. */
  startFailure?: string;
};

// The body reads `useWide`, so it renders inside DetailPage's page mode. The list loads out here
// because the top bar counts its Needs-you cards.
export function DesignPage(props: DesignPageProps) {
  const { list, failed } = useEpicList(props.fetchList, useConnectionContext());
  return (
    <DetailPage active="epics" fill mode="wide" waiting={list ? waitingCount(list.epics ?? [], props.address) : 0}>
      <DesignPageBody {...props} list={list} failed={failed} />
    </DetailPage>
  );
}

function DesignPageBody({
  address,
  deps = {},
  startFailure,
  list,
  failed,
}: DesignPageProps & { list: EpicList | null; failed: boolean }) {
  const wide = useWide();

  return (
    <>
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <DesignBody
          address={address}
          wide={wide}
          deps={deps}
          epics={list.epics ?? []}
          startFailure={startFailure}
        />
      )}
    </>
  );
}
