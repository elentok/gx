import { useEffect, useState } from "react";
import { RouterProvider } from "react-router";
import {
  ConnectionContext,
  useConnection,
  useConnectionState,
  type Connection,
  type ConnectionDeps,
} from "./connection";
import { pruneDrafts } from "./drafts";
import { routeHashLinkClicks } from "./nav/hashLinks";
import { createAppRouter } from "./routes";
import { NodeMissingToast } from "./NodeMissingToast";
import { SkillsProblemToast } from "./SkillsProblemToast";
import type { FetchDetail } from "./ticket/detail";

// A cached snapshot may predate sessions that exist now, so only a live one may delete drafts.
function usePruneDraftsOnLiveSnapshot(conn: Connection) {
  const { snapshot, liveSnapshot } = useConnectionState(conn);
  useEffect(() => {
    if (liveSnapshot && snapshot) pruneDrafts(snapshot.sessions.map((s) => s.id));
  }, [snapshot, liveSnapshot]);
}

export function App({ deps, fetchDetail }: { deps?: ConnectionDeps; fetchDetail?: FetchDetail }) {
  const conn = useConnection(deps);
  usePruneDraftsOnLiveSnapshot(conn);
  const [router] = useState(() => createAppRouter(fetchDetail));
  useEffect(() => routeHashLinkClicks(router), [router]);
  return (
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
      <SkillsProblemToast />
      <NodeMissingToast />
    </ConnectionContext.Provider>
  );
}
