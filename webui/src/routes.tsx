import { createHashRouter, redirect, useParams, type RouteObject } from "react-router";
import { useConnectionContext, useConnectionState } from "./connection";
import type { FetchChat } from "./design/chat";
import { DesignPage } from "./design/DesignPage";
import type { PostPrompt } from "./design/prompt";
import type { FetchEpicDetail } from "./epic/detail";
import { EpicPage } from "./epic/EpicPage";
import { EpicsPage } from "./epics/EpicsPage";
import type { FetchEpicList } from "./epics/list";
import { QueueScreen } from "./queue/QueueScreen";
import type { FetchDetail } from "./ticket/detail";
import { TicketPage } from "./ticket/TicketPage";

function QueueRoute() {
  const conn = useConnectionContext();
  return <QueueScreen state={useConnectionState(conn)} onChanged={conn.refresh} />;
}

function TicketRoute({ fetchDetail }: { fetchDetail?: FetchDetail }) {
  // An address holds a slash (project:epic/NN), so it arrives as the splat.
  const address = useParams()["*"] ?? "";
  return <TicketPage address={address} fetchDetail={fetchDetail} />;
}

function EpicRoute({ fetchDetail }: { fetchDetail?: FetchEpicDetail }) {
  return <EpicPage address={useParams()["*"] ?? ""} fetchDetail={fetchDetail} />;
}

function DesignRoute({ fetchers }: { fetchers: RouteFetchers }) {
  // A Design's address holds a colon (project:epic), so it arrives as the splat; the bare route has none.
  return (
    <DesignPage
      address={useParams()["*"] || undefined}
      fetchList={fetchers.epicList}
      fetchDetail={fetchers.epic}
      fetchChat={fetchers.chat}
      post={fetchers.prompt}
    />
  );
}

export interface RouteFetchers {
  ticket?: FetchDetail;
  epic?: FetchEpicDetail;
  epicList?: FetchEpicList;
  chat?: FetchChat;
  prompt?: PostPrompt;
}

export function appRoutes(fetchers: RouteFetchers = {}): RouteObject[] {
  return [
    { path: "/q", element: <QueueRoute /> },
    { path: "/epics", element: <EpicsPage fetchList={fetchers.epicList} /> },
    { path: "/designs", element: <DesignRoute fetchers={fetchers} /> },
    { path: "/d/*", element: <DesignRoute fetchers={fetchers} /> },
    { path: "/t/*", element: <TicketRoute fetchDetail={fetchers.ticket} /> },
    { path: "/e/*", element: <EpicRoute fetchDetail={fetchers.epic} /> },
    // "/" and an unknown hash go to the Queue.
    { path: "*", loader: () => redirect("/q") },
  ];
}

export const createAppRouter = (fetchDetail?: FetchDetail) => createHashRouter(appRoutes({ ticket: fetchDetail }));
