import { useEffect, useState } from "react"
import { RouterProvider } from "react-router"
import {
  ConnectionContext,
  useConnection,
  useConnectionState,
  type Connection,
  type ConnectionDeps,
} from "./connection"
import { pruneDrafts } from "./drafts"
import { routeHashLinkClicks } from "./nav/hashLinks"
import { useRestHeartbeat } from "./rest"
import { RestScreen } from "./RestScreen"
import { createAppRouter } from "./routes"
import { trackRouter } from "./routeSpans"
import { NodeMissingToast } from "./NodeMissingToast"
import { RestWarningToast } from "./RestWarningToast"
import { SkillsProblemToast } from "./SkillsProblemToast"
import type { FetchDetail } from "./ticket/detail"

// A cached snapshot may predate sessions that exist now, so only a live one may delete drafts.
function usePruneDraftsOnLiveSnapshot(conn: Connection) {
  const { snapshot, liveSnapshot } = useConnectionState(conn)
  useEffect(() => {
    // A null list means the server could not read the sessions folder, not that there are none.
    if (liveSnapshot && snapshot?.sessions) pruneDrafts(snapshot.sessions.map((s) => s.id))
  }, [snapshot, liveSnapshot])
}

// The hook reads the connection context, so it mounts inside the provider.
function RestHeartbeat() {
  useRestHeartbeat()
  return null
}

export function App({ deps, fetchDetail }: { deps?: ConnectionDeps; fetchDetail?: FetchDetail }) {
  const conn = useConnection(deps)
  usePruneDraftsOnLiveSnapshot(conn)
  const [router] = useState(() => createAppRouter(fetchDetail))
  useEffect(() => routeHashLinkClicks(router), [router])
  useEffect(() => trackRouter(router), [router])
  return (
    <ConnectionContext.Provider value={conn}>
      <RestHeartbeat />
      <RouterProvider router={router} />
      <RestScreen />
      <SkillsProblemToast />
      <NodeMissingToast />
      <RestWarningToast />
    </ConnectionContext.Provider>
  )
}
