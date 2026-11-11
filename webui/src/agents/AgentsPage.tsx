import { Box, Heading, Kbd, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { useLocation, useNavigate, useParams } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import type { FetchChat } from "../design/chat";
import { useFocusReport } from "../design/focus";
import type { FetchScreen } from "../design/screen";
import { DetailPage } from "../page/DetailPage";
import { openNewAgent } from "../shell/newItemOpen";
import { Button } from "../ui/button/Button";
import { FullScreenLoader } from "../ui/loader/FullScreenLoader";
import { useWide } from "../useWide";
import { SessionView } from "./SessionView";

import { readLast, writeLast } from "./lastAgent";

// How long a just-created session may take to reach the snapshot before we call it gone.
const CREATING_GRACE_MS = 10_000;

function GoneNotice({ id }: { id: string }) {
  return (
    <>
      <Text fontWeight="600">Session not found</Text>
      <Text color="text.muted">{id} no longer exists.</Text>
    </>
  );
}

function EmptyState({ empty, onNew }: { empty: boolean; onNew: () => void }) {
  return (
    <Box h="100%" display="flex" flexDirection="column" alignItems="center" justifyContent="center" gap="3" textAlign="center">
      <Heading as="h3" fontFamily="condensed" textTransform="uppercase" fontSize="2xl">
        {empty ? "No sessions yet" : "Pick a session"}
      </Heading>
      <Text color="text.muted" maxW="42ch">
        {empty ? (
          "Sessions you start here or in the gx Agents tab show up in the sidebar."
        ) : (
          <>
            Choose a session in the sidebar, or press <Kbd bg="surface" color="text.muted" borderWidth="1px" borderColor="border">⌘K</Kbd> to search.
          </>
        )}
      </Text>
      <Button type="button" emphasis="secondary" size="sm" onClick={onNew}>
        New session
      </Button>
    </Box>
  );
}

// The snapshot is cached by snapshotCache, so a reload repaints from it before the stream lands.
// The cache may predate the session list, so only a live snapshot may redirect or call a label gone.
export function AgentsPage({ fetchChat, fetchScreen }: { fetchChat?: FetchChat; fetchScreen?: FetchScreen }) {
  const selected = useParams()["*"] || undefined;
  const wide = useWide();
  const navigate = useNavigate();
  const conn = useConnectionContext();
  const { snapshot, liveSnapshot, streams } = useConnectionState(conn);
  const sessions = snapshot?.sessions;
  const session = selected ? sessions?.find((s) => s.id === selected) : undefined;
  const listed = !!session;
  const justCreated = !!(useLocation().state as { creating?: boolean } | null)?.creating;
  const [graceOver, setGraceOver] = useState(false);
  useEffect(() => {
    setGraceOver(false);
    if (!justCreated) return;
    const t = setTimeout(() => setGraceOver(true), CREATING_GRACE_MS);
    return () => clearTimeout(t);
  }, [justCreated, selected]);
  const creating = justCreated && !listed && !graceOver;
  const gone = !!selected && !!sessions && liveSnapshot && !listed && !creating;
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
    <DetailPage title={{ kind: "Agents" }} fill>
      {!sessions ? (
        <FullScreenLoader label="Loading agents" />
      ) : (
        <Box as="main" flex="1" minH="0" minW="0">
          {creating && <FullScreenLoader label="Creating chat…" />}
          {gone && <GoneNotice id={selected!} />}
          {!selected && <EmptyState empty={sessions.length === 0} onNew={openNewAgent} />}
          {session && <SessionView session={session} fetchChat={fetchChat} fetchScreen={fetchScreen} />}
        </Box>
      )}
    </DetailPage>
  );
}
