import { Box, Heading, Kbd, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { useLocation, useNavigate, useParams } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import type { FetchChat } from "../design/chat"
import { useFocusReport } from "../design/focus"
import type { FetchScreen } from "../design/screen"
import { DetailPage } from "../page/DetailPage"
import { StateHeader } from "../page/StateHeader"
import { openNewAgent } from "../shell/newItemOpen"
import { Button } from "../ui/button/Button"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useReadyWhen } from "../useReady"
import { useWide } from "../useWide"
import { parseAgentSplat } from "../hrefs"
import { SessionView } from "./SessionView"
import { SubagentView } from "./SubagentView"
import type { PostSubagentWatch } from "./subagentWatch"
import type { FetchSubagentChat } from "./useSubagentChat"

import { readLast, writeLast } from "./lastAgent"

// How long a just-created session may take to reach the snapshot before we call it gone.
const CREATING_GRACE_MS = 10_000

function GoneNotice({ id }: { id: string }) {
  return (
    <>
      <Text fontWeight="600">Agent not found</Text>
      <Text color="text.muted">{id} no longer exists.</Text>
    </>
  )
}

function EmptyState({ empty, onNew }: { empty: boolean; onNew: () => void }) {
  return (
    <Box
      h="100%"
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      gap="3"
      textAlign="center"
    >
      <Heading as="h3" fontFamily="condensed" textTransform="uppercase" fontSize="2xl">
        {empty ? "No chats yet" : "Pick a chat"}
      </Heading>
      <Text color="text.muted" maxW="42ch">
        {empty ? (
          "Chats you start here or in the gx Agents tab show up in the sidebar."
        ) : (
          <>
            Choose a chat in the sidebar, or press{" "}
            <Kbd bg="surface" color="text.muted" borderWidth="1px" borderColor="border">
              ⌘K
            </Kbd>{" "}
            to search.
          </>
        )}
      </Text>
      <Button type="button" emphasis="secondary" size="sm" onClick={onNew}>
        New chat
      </Button>
    </Box>
  )
}

// The snapshot is cached by snapshotCache, so a reload repaints from it before the stream lands.
// The cache may predate the session list, so only a live snapshot may redirect or call a label gone.
export function AgentsPage({
  fetchChat,
  fetchScreen,
  fetchSubagentChat,
  postSubagentWatch,
}: {
  fetchChat?: FetchChat
  fetchScreen?: FetchScreen
  fetchSubagentChat?: FetchSubagentChat
  postSubagentWatch?: PostSubagentWatch
}) {
  const { session: sessionId, callId } = parseAgentSplat(useParams()["*"] ?? "")
  const selected = sessionId || undefined
  const wide = useWide()
  const navigate = useNavigate()
  const conn = useConnectionContext()
  const { snapshot, liveSnapshot, streams } = useConnectionState(conn)
  const sessions = snapshot?.sessions
  const session = selected ? sessions?.find((s) => s.id === selected) : undefined
  const listed = !!session
  const justCreated = !!(useLocation().state as { creating?: boolean } | null)?.creating
  const [graceOverFor, setGraceOverFor] = useState<string>()
  const graceOver = graceOverFor !== undefined
  useEffect(() => {
    setGraceOverFor(undefined)
    if (!justCreated) return
    const t = setTimeout(() => setGraceOverFor(selected ?? ""), CREATING_GRACE_MS)
    return () => clearTimeout(t)
  }, [justCreated, selected])
  const creating = justCreated && !listed && !graceOver
  const gone = !!selected && !!sessions && liveSnapshot && !listed && !creating
  useFocusReport(conn, listed ? selected : undefined, streams)
  useReadyWhen(
    !!sessions,
    gone || (!selected && sessions?.length === 0) ? "empty" : "ready",
    liveSnapshot ? "network" : "cache",
  )
  useEffect(() => {
    if (!sessions || !liveSnapshot) return
    if (listed) writeLast(selected!)
    else if (!selected && wide) {
      const last = readLast()
      if (last && sessions.some((s) => s.id === last))
        navigate(`/agents/${last}`, { replace: true })
    }
  }, [sessions, liveSnapshot, listed, selected, wide, navigate])
  return (
    <DetailPage title={{ kind: "Agents" }} fill>
      {!sessions ? (
        <>
          <StateHeader title="Agents" />
          <FullScreenLoader label="Loading agents" />
        </>
      ) : (
        <Box as="main" flex="1" minH="0" minW="0">
          {!session && (
            <Box h="100%" display="flex" flexDirection="column">
              <StateHeader title="Agents" />
              <Box flex="1" minH="0">
                {creating && <FullScreenLoader label="Starting chat…" />}
                {gone && <GoneNotice id={selected!} />}
                {!selected && <EmptyState empty={sessions.length === 0} onNew={openNewAgent} />}
              </Box>
            </Box>
          )}
          {session && callId ? (
            <SubagentView
              session={session}
              callId={callId}
              fetchSubagentChat={fetchSubagentChat}
              postWatch={postSubagentWatch}
            />
          ) : (
            session && (
              <SessionView session={session} fetchChat={fetchChat} fetchScreen={fetchScreen} />
            )
          )}
        </Box>
      )}
    </DetailPage>
  )
}
