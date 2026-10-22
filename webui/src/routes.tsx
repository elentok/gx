import { createHashRouter, redirect, useParams, type RouteObject } from "react-router";
import { useConnectionContext, useConnectionState } from "./connection";
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

export function appRoutes(
  fetchDetail?: FetchDetail,
  fetchEpicDetail?: FetchEpicDetail,
  fetchEpicList?: FetchEpicList,
): RouteObject[] {
  return [
    { path: "/q", element: <QueueRoute /> },
    { path: "/epics", element: <EpicsPage fetchList={fetchEpicList} /> },
    { path: "/t/*", element: <TicketRoute fetchDetail={fetchDetail} /> },
    { path: "/e/*", element: <EpicRoute fetchDetail={fetchEpicDetail} /> },
    // "/" and an unknown hash go to the Queue.
    { path: "*", loader: () => redirect("/q") },
  ];
}

export const createAppRouter = (fetchDetail?: FetchDetail) => createHashRouter(appRoutes(fetchDetail));
