import { useState } from "react";
import { RouterProvider } from "react-router";
import { ConnectionContext, useConnection, type ConnectionDeps } from "./connection";
import { createAppRouter } from "./routes";
import type { FetchDetail } from "./ticket/detail";

export function App({ deps, fetchDetail }: { deps?: ConnectionDeps; fetchDetail?: FetchDetail }) {
  const conn = useConnection(deps);
  const [router] = useState(() => createAppRouter(fetchDetail));
  return (
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>
  );
}
