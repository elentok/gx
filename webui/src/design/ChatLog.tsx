import { Box, Text } from "@chakra-ui/react";
import { useConnectionContext, useConnectionState } from "../connection";
import type { SessionBackgroundTask, SessionChatMessage, SessionInfo } from "../generated/server";
import { Markdown } from "../markdown/Markdown";
import { useSessionChat, type FetchChat } from "./chat";
import { Composer } from "./Composer";
import type { PostPrompt } from "./prompt";

const KIND_NAMES: Record<string, string> = { shell: "shell", monitor: "Monitor", agent: "subagent" };

function toolsLine(m: SessionChatMessage, latest: boolean) {
  const n = m.tool_count ?? 0;
  const line = `· ${n} tool ${n === 1 ? "call" : "calls"}`;
  return latest && m.last_tool ? `${line}, ${m.last_tool}` : line;
}

function Entry({ m, tool }: { m: SessionChatMessage; tool: boolean }) {
  if (m.kind === "compacted") {
    return (
      <Text role="separator" textAlign="center" fontSize="xs" color="text.dim" borderBlockStart="1px solid" borderColor="border" pt="1">
        Context compacted
      </Text>
    );
  }
  if (m.kind === "tools") {
    return (
      <Text fontSize="xs" color="text.dim">
        {toolsLine(m, tool)}
      </Text>
    );
  }
  if (m.kind === "prompt") {
    return (
      <Box alignSelf="flex-end" maxW="85%" bg="bg.subtle" borderRadius="lg" px="3" py="2" fontSize="sm" whiteSpace="pre-wrap">
        {m.text}
      </Box>
    );
  }
  if (m.kind === "command") {
    return (
      <Text
        as="span"
        alignSelf="flex-end"
        fontSize="xs"
        color="text.muted"
        border="1px solid"
        borderColor="border"
        borderRadius="full"
        px="2"
      >
        {m.text}
      </Text>
    );
  }
  return <Markdown fontSize="sm">{m.text}</Markdown>;
}

function BackgroundStrip({ tasks }: { tasks: SessionBackgroundTask[] }) {
  return (
    <Box role="list" aria-label="Running in background" display="flex" flexWrap="wrap" gap="2" fontSize="xs" color="text.muted">
      {tasks.map((t, i) => (
        <Box key={i} role="listitem" border="1px solid" borderColor="border" borderRadius="md" px="2" py="0.5">
          {KIND_NAMES[t.kind] ?? t.kind}: {t.label}
        </Box>
      ))}
    </Box>
  );
}

// The chat of a Claude session. Other harnesses have no chat, so DesignChat never mounts this for them.
function ChatLog({
  session,
  fetchChat,
  post,
}: {
  session: SessionInfo;
  fetchChat?: FetchChat;
  post?: PostPrompt;
}) {
  const conn = useConnectionContext();
  const { detail: chat, refetch } = useSessionChat(session.id, conn, fetchChat);
  const connected = useConnectionState(conn).connected;
  if (!chat) return null;
  const working = session.status.state === "working";
  return (
    <Box display="flex" flexDirection="column" gap="3">
      {chat.truncated && (
        <Text fontSize="xs" color="text.dim" textAlign="center">
          Earlier messages hidden
        </Text>
      )}
      {chat.messages.map((m, i) => (
        <Entry key={m.id || i} m={m} tool={working && i === chat.messages.length - 1} />
      ))}
      {chat.background.length > 0 && <BackgroundStrip tasks={chat.background} />}
      <Composer
        session={session}
        lastMessage={chat.messages.at(-1)?.id}
        next={chat.next}
        connected={connected}
        post={post}
        onStale={refetch}
      />
    </Box>
  );
}

/** The center column of a Design: its session's chat, or an empty chat when it has none. */
export function DesignChat({
  session,
  fetchChat,
  post,
}: {
  session?: SessionInfo;
  fetchChat?: FetchChat;
  post?: PostPrompt;
}) {
  if (!session) return null;
  if (session.harness !== "claude") {
    return (
      <Text color="text.muted" fontSize="sm">
        Chat works for Claude sessions only. Open it in gx, Agents tab.
      </Text>
    );
  }
  return <ChatLog key={session.id} session={session} fetchChat={fetchChat} post={post} />;
}
