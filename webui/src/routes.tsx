import { createHashRouter, Navigate, redirect, useLocation, useParams, type RouteObject } from "react-router";
import type { FetchChat } from "./design/chat";
import { DesignPage } from "./design/DesignPage";
import type { DeleteDesign, OpenDesign, ResumeSession, StopSession } from "./design/open";
import type { PostFocus } from "./design/focus";
import type { PostPrompt } from "./design/prompt";
import type { FetchScreen } from "./design/screen";
import type { FetchEpicDetail } from "./epic/detail";
import { EpicPage } from "./epic/EpicPage";
import { EpicsPage } from "./epics/EpicsPage";
import type { FetchEpicList } from "./epics/list";
import type { FetchStats } from "./stats/stats";
import { StatsPage } from "./stats/StatsPage";
import type { FetchDetail } from "./ticket/detail";
import { TicketPage } from "./ticket/TicketPage";

function TicketRoute({ fetchDetail, fetchScreen }: { fetchDetail?: FetchDetail; fetchScreen?: FetchScreen }) {
  // An address holds a slash (project:epic/NN), so it arrives as the splat.
  const address = useParams()["*"] ?? "";
  return <TicketPage address={address} fetchDetail={fetchDetail} fetchScreen={fetchScreen} />;
}

function EpicRoute({
  fetchDetail,
  fetchList,
  openDesign,
}: {
  fetchDetail?: FetchEpicDetail;
  fetchList?: FetchEpicList;
  openDesign?: OpenDesign;
}) {
  return <EpicPage address={useParams()["*"] ?? ""} fetchDetail={fetchDetail} fetchList={fetchList} openDesign={openDesign} />;
}

function DesignRoute({ fetchers }: { fetchers: RouteFetchers }) {
  // A Design's address holds a colon (project:epic), so it arrives as the splat.
  const address = useParams()["*"];
  const startFailure = (useLocation().state as { startFailure?: string } | null)?.startFailure;
  if (!address) return <Navigate to="/epics" replace />;
  return (
    <DesignPage
      address={address}
      startFailure={startFailure}
      fetchList={fetchers.epicList}
      deps={{
        fetchDetail: fetchers.epic,
        fetchChat: fetchers.chat,
        post: fetchers.prompt,
        open: fetchers.openDesign,
        remove: fetchers.deleteDesign,
        stop: fetchers.stop,
        resume: fetchers.resume,
        fetchScreen: fetchers.screen,
        focus: fetchers.focus,
      }}
    />
  );
}

export interface RouteFetchers {
  openDesign?: OpenDesign;
  deleteDesign?: DeleteDesign;
  stop?: StopSession;
  resume?: ResumeSession;
  screen?: FetchScreen;
  ticketScreen?: FetchScreen;
  ticket?: FetchDetail;
  epic?: FetchEpicDetail;
  epicList?: FetchEpicList;
  stats?: FetchStats;
  chat?: FetchChat;
  prompt?: PostPrompt;
  focus?: PostFocus;
}

export function appRoutes(fetchers: RouteFetchers = {}): RouteObject[] {
  return [
    { path: "/epics", element: <EpicsPage fetchList={fetchers.epicList} openDesign={fetchers.openDesign} /> },
    { path: "/stats", element: <StatsPage fetchStats={fetchers.stats} fetchEpics={fetchers.epicList} /> },
    { path: "/d/*", element: <DesignRoute fetchers={fetchers} /> },
    { path: "/t/*", element: <TicketRoute fetchDetail={fetchers.ticket} fetchScreen={fetchers.ticketScreen} /> },
    { path: "/e/*", element: <EpicRoute fetchDetail={fetchers.epic} fetchList={fetchers.epicList} openDesign={fetchers.openDesign} /> },
    // "/", the retired "/q" and "/designs", and an unknown hash go to the Epics page.
    { path: "*", loader: () => redirect("/epics") },
  ];
}

export const createAppRouter = (fetchDetail?: FetchDetail) => createHashRouter(appRoutes({ ticket: fetchDetail }));
