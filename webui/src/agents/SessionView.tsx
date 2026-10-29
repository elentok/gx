import { Box, Text } from "@chakra-ui/react";
import { useConnectionContext, useConnectionState } from "../connection";
import { useSessionChat, type FetchChat } from "../design/chat";
import { DesignChat, type ChatOptions } from "../design/ChatLog";
import { Composer } from "../design/Composer";
import { promptable } from "../design/view";
import type { SessionInfo } from "../generated/server";
import { Chip } from "../ui/chip/Chip";
import { statusMark } from "./AgentSidebar";
import { SessionSwitcher } from "./SessionSwitcher";

// No question sheet, no step names, no "gx stopped", no Done state, and no Restore or New session.
const AGENT_OPTIONS: ChatOptions = { sheet: false, steps: false, gxStopped: false, done: false, restore: false, composer: false };

const noop = () => {};

function SessionHeader({ session: s, sessions }: { session: SessionInfo; sessions: SessionInfo[] }) {
  const { glyph, tone } = statusMark(s);
  return (
    <Box as="header" display="flex" alignItems="center" flexWrap="wrap" gap="2" mb="3">
      <Text as="span" role="img" aria-label="status" color={`tone.${tone}.fg`}>
        {glyph}
      </Text>
      <SessionSwitcher name={s.name || s.id} sessions={sessions} current={s.id} />
      <Chip tone={tone}>{s.status.state}</Chip>
      {s.harness !== "claude" && <Chip>{s.harness}</Chip>}
      <Text as="span" fontSize="sm" color="text.dim">
        {s.dir}
      </Text>
      {s.ticket && (
        <Text as="span" fontSize="sm" color="text.dim">
          {s.ticket}
        </Text>
      )}
    </Box>
  );
}

/** Why Send is off, first match wins; nothing when the composer may send. */
function sendReason(s: SessionInfo, connected: boolean): string | undefined {
  const state = s.status.state;
  if (!connected) return "Offline. Send turns on when gx reconnects.";
  if (s.harness !== "claude") return "Sending works for Claude sessions only";
  if (state === "exited") return "Exited. Restore it in gx, Agents tab.";
  if (!promptable(s)) {
    const label = state.charAt(0).toUpperCase() + state.slice(1).replace("-", " ");
    return `${label}. Send turns on when the session is idle.`;
  }
}

/** The main column for a selected session: its header, chat and composer. */
export function SessionView({
  session,
  sessions,
  fetchChat,
}: {
  session: SessionInfo;
  sessions: SessionInfo[];
  fetchChat?: FetchChat;
}) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const claude = session.harness === "claude";
  const { detail: chat, refetch } = useSessionChat(claude ? session.id : undefined, conn, fetchChat);
  return (
    <Box display="flex" flexDirection="column" height="100%" minH="0">
      <SessionHeader session={session} sessions={sessions} />
      {claude && chat && chat.messages.length === 0 && (
        <Text color="text.muted" mb="3">
          No messages yet
        </Text>
      )}
      <DesignChat
        session={session}
        chat={chat}
        refetch={refetch}
        actions={{ address: "" }}
        questions={false}
        onQuestions={noop}
        options={AGENT_OPTIONS}
      />
      <Composer
        design={false}
        session={session}
        connected={connected && claude}
        lastMessage={chat?.messages.at(-1)?.id}
        onStale={refetch}
        reason={sendReason(session, connected)}
      />
    </Box>
  );
}
