import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicDetail, type FetchEpicDetail } from "../epic/detail";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { BackLink } from "../nav/BackLink";
import { JumpMenu } from "../nav/JumpMenu";
import { DetailPage } from "../page/DetailPage";
import { Spinner } from "../ui/spinner/Spinner";
import type { FetchChat } from "./chat";
import { useFocusReport, type PostFocus } from "./focus";
import { DesignChat } from "./ChatLog";
import type { DeleteDesign, OpenDesign, ResumeSession, StopSession } from "./open";
import type { PostPrompt } from "./prompt";
import { ImplLink, Progress, showsImplLink } from "./Progress";
import type { FetchScreen } from "./screen";
import { DeleteDesignButton, Failure, SessionHeader, StartDesignButton } from "./SessionControls";
import type { SessionInfo } from "../generated/server";
import { useWide } from "./useWide";
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
      {session ? (
        <SessionHeader
          address={address}
          shown={session}
          live={live}
          onSelect={onSelect}
          open={deps.open}
          stop={deps.stop}
        />
      ) : (
        <StartDesignButton address={address} open={deps.open} />
      )}
      {deletable && live.length === 0 && (
        <DeleteDesignButton address={address} remove={deps.remove} onDeleted={() => navigate("/epics")} />
      )}
    </Box>
  );
}

// The title row and the Chat and Progress columns of one Design. Progress reads the Design's epic detail.
function DesignBody({
  address,
  wide,
  deps,
  fetchList,
  startFailure,
}: {
  address: string;
  wide: boolean;
  deps: DesignDeps;
  fetchList?: FetchEpicList;
  startFailure?: string;
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
          deps={deps}
          onSelect={setSelected}
          startFailure={startFailure}
          deletable={detail?.deletable}
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
              address,
              open: deps.open,
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

export function DesignPage({
  address,
  fetchList,
  deps = {},
  startFailure,
}: {
  address: string;
  fetchList?: FetchEpicList;
  deps?: DesignDeps;
  /** Why the session did not start, when the New Design modal created the map but the start failed. */
  startFailure?: string;
}) {
  const conn = useConnectionContext();
  const { list, failed } = useEpicList(fetchList, conn);

  const wide = useWide();

  return (
    <DetailPage active="epics" fill>
      {failed ? (
        <Text color="text.muted">Could not load Designs</Text>
      ) : !list ? (
        <Spinner label="Loading Designs" />
      ) : (
        <DesignBody
          address={address}
          wide={wide}
          deps={deps}
          fetchList={fetchList}
          startFailure={startFailure}
        />
      )}
    </DetailPage>
  );
}
