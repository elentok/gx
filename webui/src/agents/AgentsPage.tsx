import { Box, Heading, Kbd, Link, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import type { FetchChat } from "../design/chat";
import { useFocusReport } from "../design/focus";
import type { FetchScreen } from "../design/screen";
import { AGENTS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { Button } from "../ui/button/Button";
import { Spinner } from "../ui/spinner/Spinner";
import { useWide } from "../useWide";
import { AgentSidebar } from "./AgentSidebar";
import type { CreateSession, FetchWorktrees } from "./newSession";
import { NewSessionForm } from "./NewSessionForm";
import { SessionSwitcher } from "./SessionSwitcher";
import { SessionView } from "./SessionView";

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
export function AgentsPage({
  fetchChat,
  fetchScreen,
  fetchTrees,
  createSession,
}: {
  fetchChat?: FetchChat;
  fetchScreen?: FetchScreen;
  fetchTrees?: FetchWorktrees;
  createSession?: CreateSession;
}) {
  const selected = useParams()["*"] || undefined;
  // The form is open while this is set; `project` is its prefill.
  const [creating, setCreating] = useState<{ project?: string } | null>(null);
  const wide = useWide();
  const navigate = useNavigate();
  const conn = useConnectionContext();
  const { snapshot, liveSnapshot, streams } = useConnectionState(conn);
  const sessions = snapshot?.sessions;
  const listed = !!selected && !!sessions?.some((s) => s.id === selected);
  const gone = !!selected && !!sessions && liveSnapshot && !listed;
  useFocusReport(conn, listed ? selected : undefined, streams);
  const selectedProject = sessions?.find((s) => s.id === selected)?.project;
  const openNew = (project = selectedProject) => setCreating({ project });
  useEffect(() => {
    if (!sessions || !liveSnapshot) return;
    if (listed) writeLast(selected!);
    else if (!selected && wide) {
      const last = readLast();
      if (last && sessions.some((s) => s.id === last)) navigate(`/agents/${last}`, { replace: true });
    }
  }, [sessions, liveSnapshot, listed, selected, wide, navigate]);
  return (
    <DetailPage title={{ kind: "Agents" }} active="agents" fill mode="wide">
      {!sessions ? (
        <Spinner label="Loading agents" />
      ) : (
        // The negative margins cancel PageContainer's padding so the sidebar fills the column edge to edge.
        <Box flex="1" minH="0" display="grid" gridTemplateRows="minmax(0, 1fr)" gridTemplateColumns={wide ? "300px 1fr" : "1fr"} mx="-4" mb="-4" borderTopWidth="1px" borderColor="border">
          {(wide || !selected) && <AgentSidebar sessions={sessions} selected={selected} onNew={openNew} />}
          {(wide || selected) && (
            <Box as="main" p={wide ? "4" : "3"} minW="0">
              {!listed && <SessionSwitcher name="Switch session" sessions={sessions} onNew={() => openNew()} />}
              {gone ? (
                <>
                  <Text fontWeight="600">Session not found</Text>
                  <Text color="text.muted">{selected} no longer exists.</Text>
                  {!wide && (
                    <Link href={AGENTS_HREF} color="tone.running.fg">
                      Back to sessions
                    </Link>
                  )}
                </>
              ) : selected ? (
                listed && (
                  <SessionView
                    session={sessions.find((s) => s.id === selected)!}
                    sessions={sessions}
                    onNew={() => openNew()}
                    fetchChat={fetchChat}
                    fetchScreen={fetchScreen}
                  />
                )
              ) : (
                <Box h="100%" display="flex" flexDirection="column" alignItems="center" justifyContent="center" gap="3" textAlign="center">
                  <Heading as="h3" fontFamily="condensed" textTransform="uppercase" fontSize="2xl">
                    {sessions.length === 0 ? "No sessions yet" : "Pick a session"}
                  </Heading>
                  <Text color="text.muted" maxW="42ch">
                    {sessions.length === 0 ? (
                      "Sessions you start here or in the gx Agents tab show up in this list."
                    ) : wide ? (
                      <>
                        Choose a session on the left, or press <Kbd bg="surface" color="text.muted" borderWidth="1px" borderColor="border">⌘K</Kbd> to search.
                      </>
                    ) : (
                      "Choose a session from the list."
                    )}
                  </Text>
                  <Button type="button" emphasis="secondary" size="sm" onClick={() => openNew()}>
                    New session
                  </Button>
                </Box>
              )}
            </Box>
          )}
        </Box>
      )}
      {creating && (
        <NewSessionForm
          prefillProject={creating.project}
          fetchTrees={fetchTrees}
          create={createSession}
          onClose={() => setCreating(null)}
          onCreated={(id) => {
            setCreating(null);
            navigate(`/agents/${id}`);
          }}
        />
      )}
    </DetailPage>
  );
}
