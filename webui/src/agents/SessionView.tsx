import { Box, Heading, Text } from "@chakra-ui/react";
import { useConnectionContext } from "../connection";
import { useSessionChat, type FetchChat } from "../design/chat";
import { DesignChat, type ChatOptions } from "../design/ChatLog";
import type { SessionInfo } from "../generated/server";
import { Chip } from "../ui/chip/Chip";
import { statusMark } from "./AgentSidebar";

// No question sheet, no step names, no "gx stopped", no Done state, and no Restore or New session.
const AGENT_OPTIONS: ChatOptions = { sheet: false, steps: false, gxStopped: false, done: false, restore: false };

const noop = () => {};

function SessionHeader({ session: s }: { session: SessionInfo }) {
  const { glyph, tone } = statusMark(s);
  return (
    <Box as="header" display="flex" alignItems="center" flexWrap="wrap" gap="2" mb="3">
      <Text as="span" role="img" aria-label="status" color={`tone.${tone}.fg`}>
        {glyph}
      </Text>
      <Heading as="h1" fontSize="lg">
        {s.name || s.id}
      </Heading>
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

/** The main column for a selected session: its header and chat. */
export function SessionView({ session, fetchChat }: { session: SessionInfo; fetchChat?: FetchChat }) {
  const conn = useConnectionContext();
  const claude = session.harness === "claude";
  const { detail: chat, refetch } = useSessionChat(claude ? session.id : undefined, conn, fetchChat);
  return (
    <Box display="flex" flexDirection="column" height="100%" minH="0">
      <SessionHeader session={session} />
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
    </Box>
  );
}
