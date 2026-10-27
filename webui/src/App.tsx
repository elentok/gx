import { useEffect, useState } from "react";
import { RouterProvider } from "react-router";
import { ConnectionContext, useConnection, type ConnectionDeps } from "./connection";
import { routeHashLinkClicks } from "./nav/hashLinks";
import { createAppRouter } from "./routes";
import { SkillsProblemToast } from "./SkillsProblemToast";
import type { FetchDetail } from "./ticket/detail";

export function App({ deps, fetchDetail }: { deps?: ConnectionDeps; fetchDetail?: FetchDetail }) {
  const conn = useConnection(deps);
  const [router] = useState(() => createAppRouter(fetchDetail));
  useEffect(() => routeHashLinkClicks(router), [router]);
  return (
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
      <SkillsProblemToast />
    </ConnectionContext.Provider>
  );
}
