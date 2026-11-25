import { Box } from "@chakra-ui/react"
import { useConnectionContext, useConnectionState } from "../connection"
import type { SessionInfo } from "../generated/server"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { SubagentMissing, SubagentPage } from "./SubagentPage"
import { useSubagentWatch, type PostSubagentWatch } from "./subagentWatch"
import { useSubagentChat, type FetchSubagentChat } from "./useSubagentChat"

/** The route's body for `/agents/<session>/subagents/<call id>`: fetch, watch while open, show. */
export function SubagentView({
  session,
  callId,
  fetchSubagentChat,
  postWatch,
}: {
  session: SessionInfo
  callId: string
  fetchSubagentChat?: FetchSubagentChat
  postWatch?: PostSubagentWatch
}) {
  const conn = useConnectionContext()
  const { streams } = useConnectionState(conn)
  const { detail, notFound, message } = useSubagentChat(session.id, callId, conn, fetchSubagentChat)
  useSubagentWatch(conn, session.id, callId, streams, postWatch)
  return (
    <Box h="100%" display="flex" flexDirection="column" overflowY="auto">
      {detail ? (
        <SubagentPage session={session} chat={detail} />
      ) : notFound ? (
        <SubagentMissing sessionId={session.id} message={message} />
      ) : (
        <FullScreenLoader label="Loading subagent" />
      )}
    </Box>
  )
}
