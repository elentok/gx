import { Box, Text } from "@chakra-ui/react";
import { useEffect } from "react";
import { useNavigate, useParams } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useFocusReport } from "../design/focus";
import { DetailPage } from "../page/DetailPage";
import { Spinner } from "../ui/spinner/Spinner";
import { useWide } from "../useWide";
import { AgentSidebar } from "./AgentSidebar";

const LAST_KEY = "gx.agents.last";

function readLast(): string | null {
  try {
    return localStorage.getItem(LAST_KEY);
  } catch {
    return null;
  }
}

function writeLast(id: string) {
  try {
    localStorage.setItem(LAST_KEY, id);
  } catch {
    // Storage is a convenience; the page works without it.
  }
}

// The snapshot is cached by snapshotCache, so a reload repaints from it before the stream lands.
// The cache may predate the session list, so only a live snapshot may redirect or call a label gone.
export function AgentsPage() {
  const selected = useParams()["*"] || undefined;
  const wide = useWide();
  const navigate = useNavigate();
  const conn = useConnectionContext();
  const { snapshot, liveSnapshot, streams } = useConnectionState(conn);
  const sessions = snapshot?.sessions;
  const listed = !!selected && !!sessions?.some((s) => s.id === selected);
  const gone = !!selected && !!sessions && liveSnapshot && !listed;
  useFocusReport(conn, listed ? selected : undefined, streams);
  useEffect(() => {
    if (!sessions || !liveSnapshot) return;
    if (listed) writeLast(selected!);
    else if (!selected && wide) {
      const last = readLast();
      if (last && sessions.some((s) => s.id === last)) navigate(`/agents/${last}`, { replace: true });
    }
  }, [sessions, liveSnapshot, listed, selected, wide, navigate]);
  return (
    <DetailPage active="agents" fill mode="wide">
      {!sessions ? (
        <Spinner label="Loading agents" />
      ) : (
        <Box flex="1" minH="0" display="grid" gridTemplateColumns={wide ? "300px 1fr" : "1fr"}>
          {(wide || !selected) && <AgentSidebar sessions={sessions} selected={selected} />}
          {(wide || selected) && (
            <Box as="main" p="4" minW="0">
              {gone ? (
                <>
                  <Text fontWeight="600">Session not found</Text>
                  <Text color="text.muted">{selected} no longer exists.</Text>
                </>
              ) : selected ? null : (
                <Text color="text.muted">{sessions.length === 0 ? "No sessions yet" : "Pick a session"}</Text>
              )}
            </Box>
          )}
        </Box>
      )}
    </DetailPage>
  );
}
