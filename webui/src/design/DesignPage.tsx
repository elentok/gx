import { Text } from "@chakra-ui/react";
import { useEffect, useRef } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicDetail, type FetchEpicDetail } from "../epic/detail";
import { EpicRing } from "../epic/EpicRing";
import { useEpicList, type EpicList, type FetchEpicList } from "../epics/list";
import { PageIcon } from "../page/PageIcon";
import { DetailPage } from "../page/DetailPage";
import { FullScreenLoader } from "../ui/loader/FullScreenLoader";
import type { FetchChat } from "./chat";
import type { PostFocus } from "./focus";
import { MapImageStrip } from "./MapImageStrip";
import type { FetchMapImages } from "./mapImages";
import type { DeleteDesign, OpenDesign, ResumeSession, StopSession } from "./open";
import type { PostAnswer, PostPrompt } from "./prompt";
import { ImplLink, Tickets, showsImplLink } from "./Progress";
import { designModel } from "../stepped/model";
import { SteppedPage } from "../stepped/SteppedPage";
import type { FetchScreen } from "./screen";
import { SectionOnHold, type EpicListRow } from "../generated/server";
import { slugOf } from "../slug";
import { useWide } from "../useWide";
import { useCardActions } from "../workspaces/itemActions";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { designSession, liveSessions } from "./view";

/** The injected fetchers and actions the Design page talks to the server through. */
export interface DesignDeps {
  fetchDetail?: FetchEpicDetail;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  answer?: PostAnswer;
  open?: OpenDesign;
  remove?: DeleteDesign;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
  fetchMapImages?: FetchMapImages;
}

// One Design in the shared stepped frame. Progress reads the Design's epic detail.
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
  const navigate = useNavigate();
  const conn = useConnectionContext();
  const { detail, refetch } = useEpicDetail(address, conn, deps.fetchDetail);
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const model = detail ? designModel(detail) : undefined;
  const name = slugOf(address);
  // The card speaks for this page only while its active (first) address is this Design.
  const card = epics.find((e) => e.addresses?.[0] === address);
  const live = liveSessions(address, sessions);
  // `deletable` depends on the live sessions, and a session stop is no ticket event.
  const liveKey = live.map((s) => s.id).join(",");
  const seenLiveKey = useRef(liveKey);
  useEffect(() => {
    if (seenLiveKey.current === liveKey) return;
    seenLiveKey.current = liveKey;
    refetch();
  }, [liveKey, refetch]);
  const { workspaces } = useWorkspaces(conn);
  const itemActions = useCardActions(card, workspaces);
  return (
    <>
      <SteppedPage
        address={address}
        title={name}
        icon={<PageIcon kind="design" />}
        wide={wide}
        model={model}
        ring={detail ? (size) => <EpicRing detail={detail} card={card} sessions={sessions} size={size} /> : undefined}
        live={live}
        current={designSession(address, sessions)}
        actions={deps}
        startLabel="Start design session"
        startFailure={startFailure}
        working={(session) => {
          // The busy row follows what the agent reported; Progress and Continue keep following the map.
          const running = detail?.steps?.find((s) => s.step === session?.reported_step) ?? model?.current;
          // Without the map detail there is no step to name, so the row reads a bare "Working".
          return {
            step: running?.label,
            title:
              running?.step === "resolve-tickets"
                ? session?.reported_title || detail?.rows.find((r) => r.current)?.title
                : undefined,
          };
        }}
        subtitleSuffix={card?.section === SectionOnHold && " · On hold"}
        chatTop={({ questions }) =>
          !questions && detail && showsImplLink(detail) && <ImplLink address={detail.impl_address!} />
        }
        logTop={<MapImageStrip address={address} fetcher={deps.fetchMapImages} />}
        ticketRows={model && <Tickets rows={model.tickets} />}
        designMenu={{ itemGroups: itemActions.groups, deletable: detail?.deletable, remove: deps.remove, onDeleted: () => navigate("/epics"), onOpenEpic: () => navigate(`/e/${address}`) }}
      />
      {itemActions.dialogs}
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
    <DetailPage fill title={{ kind: "Design", name: slugOf(props.address) }}>
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
        <FullScreenLoader label="Loading Designs" />
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
