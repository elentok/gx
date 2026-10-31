import { Box } from "@chakra-ui/react";
import { ChatMessages, WorkingRow } from "../design/ChatCore";
import { FailureBanner, visibleFailure } from "../design/FailureBanner";
import type { SessionChatResponse, SessionInfo } from "../generated/server";

/** The chat of a Claude or Codex session on the Agents page: the messages, a plain Working row and the failure notice. */
export function AgentChat({
  session,
  chat,
  refetch,
}: {
  session: SessionInfo;
  chat: SessionChatResponse;
  refetch: () => void;
}) {
  const busy = session.status.state === "working" && !session.status.background;
  const failure = visibleFailure(session.failure);
  return (
    <Box display="flex" flexDirection="column" gap="3" flex="1" minH="0">
      <ChatMessages chat={chat} busy={busy} sessionId={session.id} working={<WorkingRow />} />
      {failure && (
        <FailureBanner
          failure={failure}
          session={session}
          lastMessage={chat.messages.at(-1)?.id}
          onStale={refetch}
          restore={false}
          address=""
        />
      )}
    </Box>
  );
}
