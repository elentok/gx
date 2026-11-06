import { useCallback, useEffect, useState } from "react";
import { useLocation } from "react-router";
import { useConnectionState, type Connection } from "../connection";
import { useEpicList, type FetchEpicList } from "../epics/list";
import type { Workspace, SessionPlacement } from "../generated/server";
import { readStoredWorkspace, resolveActive, writeStoredWorkspace } from "./active";

/** The ordered workspaces, the session placements and the total Needs-you count of the connection snapshot. */
export function useWorkspaces(conn: Connection): { workspaces: Workspace[]; placements: SessionPlacement[]; needsYouTotal: number } {
  const snapshot = useConnectionState(conn).snapshot;
  return { workspaces: snapshot?.workspaces ?? [], placements: snapshot?.placements ?? [], needsYouTotal: snapshot?.needs_you_total ?? 0 };
}

/** The workspace the sidebar shows, kept per browser; the server stores nothing. */
export function useActiveWorkspace(conn: Connection, fetcher?: FetchEpicList): { active: string; setActive: (name: string) => void } {
  const { workspaces, placements } = useWorkspaces(conn);
  const { list } = useEpicList(fetcher, conn);
  const route = useLocation().pathname;
  const [stored, setStored] = useState(readStoredWorkspace);

  const rows = list?.epics;
  const loaded = workspaces.length > 0 && rows !== undefined;
  const active = loaded ? resolveActive({ route, rows, placements, workspaces, stored }) : (stored ?? "");

  // Persist what resolved, so an opened item's workspace sticks and a vanished one falls back to Default at once.
  useEffect(() => {
    if (!loaded || active === stored) return;
    writeStoredWorkspace(active);
    setStored(active);
  }, [loaded, active, stored]);

  const setActive = useCallback((name: string) => {
    writeStoredWorkspace(name);
    setStored(name);
  }, []);

  return { active, setActive };
}
