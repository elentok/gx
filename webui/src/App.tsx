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
import { useAppBadge } from "./push/appBadge"
import { refreshPushDevice } from "./push/deviceState"
import { runOpenChecks } from "./push/openChecks"
import { routeHashLinkClicks } from "./nav/hashLinks"
import { setTapRouter } from "./pwa/notificationTap"
import { useRestHeartbeat } from "./rest"
import { RestScreen } from "./RestScreen"
import { createAppRouter } from "./routes"
import { trackRouter } from "./routeSpans"
import { NodeMissingToast } from "./NodeMissingToast"
import { NotificationToast } from "./NotificationToast"
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

function AppBadge() {
  useAppBadge()
  return null
}

export function App({ deps, fetchDetail }: { deps?: ConnectionDeps; fetchDetail?: FetchDetail }) {
  const conn = useConnection(deps)
  usePruneDraftsOnLiveSnapshot(conn)
  const [router] = useState(() => createAppRouter(fetchDetail))
  useEffect(() => routeHashLinkClicks(router), [router])
  useEffect(() => setTapRouter(router), [router])
  useEffect(() => trackRouter(router), [router])
  useEffect(() => {
    void runOpenChecks().finally(refreshPushDevice)
  }, [])
  return (
    <ConnectionContext.Provider value={conn}>
      <RestHeartbeat />
      <AppBadge />
      <RouterProvider router={router} />
      <RestScreen />
      <SkillsProblemToast />
      <NodeMissingToast />
      <RestWarningToast />
      <NotificationToast />
    </ConnectionContext.Provider>
  )
}
