import { createHashRouter, useParams, type RouteObject } from "react-router";
import { useConnectionContext, useConnectionState } from "./connection";
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

export function appRoutes(fetchDetail?: FetchDetail): RouteObject[] {
  return [
    { path: "/", element: <QueueRoute /> },
    { path: "/t/*", element: <TicketRoute fetchDetail={fetchDetail} /> },
    // An unknown hash shows the Queue.
    { path: "*", element: <QueueRoute /> },
  ];
}

export const createAppRouter = (fetchDetail?: FetchDetail) => createHashRouter(appRoutes(fetchDetail));
