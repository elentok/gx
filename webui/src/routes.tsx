import { createHashRouter, useParams, type RouteObject } from "react-router";
import { useConnectionContext, useConnectionState } from "./connection";
import type { FetchEpicDetail } from "./epic/detail";
import { EpicPage } from "./epic/EpicPage";
import { QueueScreen } from "./queue/QueueScreen";
import type { FetchDetail } from "./ticket/detail";
import { TicketPage } from "./ticket/TicketPage";

function QueueRoute() {
  return <QueueScreen state={useConnectionState(useConnectionContext())} />;
}

function TicketRoute({ fetchDetail }: { fetchDetail?: FetchDetail }) {
  // An address holds a slash (project:epic/NN), so it arrives as the splat.
  const address = useParams()["*"] ?? "";
  return <TicketPage address={address} fetchDetail={fetchDetail} />;
}

function EpicRoute({ fetchDetail }: { fetchDetail?: FetchEpicDetail }) {
  return <EpicPage address={useParams()["*"] ?? ""} fetchDetail={fetchDetail} />;
}

export function appRoutes(fetchDetail?: FetchDetail, fetchEpicDetail?: FetchEpicDetail): RouteObject[] {
  return [
    { path: "/", element: <QueueRoute /> },
    { path: "/t/*", element: <TicketRoute fetchDetail={fetchDetail} /> },
    { path: "/e/*", element: <EpicRoute fetchDetail={fetchEpicDetail} /> },
    // An unknown hash shows the Queue.
    { path: "*", element: <QueueRoute /> },
  ];
}

export const createAppRouter = (fetchDetail?: FetchDetail) => createHashRouter(appRoutes(fetchDetail));
