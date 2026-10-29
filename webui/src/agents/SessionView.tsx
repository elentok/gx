import { Box, Link, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { useSessionChat, type FetchChat } from "../design/chat";
import { DesignChat, type ChatOptions } from "../design/ChatLog";
import { Composer } from "../design/Composer";
import { FailureBanner, needsTranscript, visibleFailure } from "../design/FailureBanner";
import { ScreenModal } from "../design/ScreenModal";
import type { FetchScreen } from "../design/screen";
import { designOf, promptable } from "../design/view";
import type { SessionInfo } from "../generated/server";
import { AGENTS_HREF, designHref } from "../hrefs";
import { useWide } from "../useWide";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { StatusMark, statusMark } from "./AgentSidebar";
import { stateLabel } from "./rules";
import { SessionSwitcher } from "./SessionSwitcher";

// No question sheet, no step names, no "gx stopped", no Done state, and no Restore or New session.
const AGENT_OPTIONS: ChatOptions = { sheet: false, steps: false, gxStopped: false, done: false, restore: false, composer: false };

const noop = () => {};


function SessionHeader({ session: s, sessions, onNew }: { session: SessionInfo; sessions: SessionInfo[]; onNew: () => void }) {
  const { tone } = statusMark(s);
  const map = designOf(s);
  const wide = useWide();
  if (!wide) {
    return (
      <Box as="header" display="flex" alignItems="center" gap="2" mb="3" pb="2" borderBottomWidth="1px" borderColor="border">
        <Link href={AGENTS_HREF} aria-label="All sessions" fontSize="2xl" lineHeight="1" px="2" color="text.muted">
          ‹
        </Link>
        <StatusMark session={s} />
        <Box display="flex" flexDirection="column" minW="0">
          <SessionSwitcher name={s.name || s.id} sessions={sessions} current={s.id} onNew={onNew} />
          <Text as="span" fontSize="sm" color="text.dim">
            {s.project} · {stateLabel(s)}
          </Text>
        </Box>
      </Box>
    );
  }
  return (
    <Box as="header" display="flex" alignItems="center" flexWrap="wrap" gap="2" mb="3" pb="2.5" borderBottomWidth="1px" borderColor="border">
      <StatusMark session={s} />
      <SessionSwitcher name={s.name || s.id} sessions={sessions} current={s.id} onNew={onNew} fontSize="26px" />
      <Chip tone={tone} fontSize="12.5px">
        {stateLabel(s)}
      </Chip>
      {s.harness !== "claude" && <Chip>{s.harness}</Chip>}
      <Text as="span" fontSize="sm" color="text.dim">
        {s.dir}
      </Text>
      {s.ticket && (
        <Text as="span" fontSize="sm" color="text.dim">
          {s.ticket}
        </Text>
      )}
      {map && (
        <Link fontSize="sm" ml="auto" color="tone.running.fg" href={designHref(map)}>
          Open design page →
        </Link>
      )}
    </Box>
  );
}

/** A non-Claude session has no transcript, so its screen is the only thing to read. */
function NoChat({ session, fetchScreen }: { session: SessionInfo; fetchScreen?: FetchScreen }) {
  const [screen, setScreen] = useState(false);
  const failure = visibleFailure(session.failure);
  return (
    <>
      {failure && !needsTranscript(failure) && (
        <FailureBanner failure={failure} session={session} restore={false} address="" />
      )}
      <Box display="flex" alignItems="center" justifyContent="space-between" gap="3" flexWrap="wrap" borderWidth="1px" borderRadius="md" px="3" py="2">
        <Text color="text.muted" fontSize="sm">
          Chat works for Claude sessions only.
        </Text>
        <Button emphasis="secondary" onClick={() => setScreen(true)}>
          Show screen
        </Button>
        {screen && <ScreenModal target={session.id} fetcher={fetchScreen} onClose={() => setScreen(false)} />}
      </Box>
    </>
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
  fetchScreen,
  onNew,
}: {
  session: SessionInfo;
  sessions: SessionInfo[];
  onNew: () => void;
  fetchChat?: FetchChat;
  fetchScreen?: FetchScreen;
}) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const claude = session.harness === "claude";
  const { detail: chat, refetch } = useSessionChat(claude ? session.id : undefined, conn, fetchChat);
  return (
    <Box display="flex" flexDirection="column" height="100%" minH="0">
      <SessionHeader session={session} sessions={sessions} onNew={onNew} />
      {claude && chat && chat.messages.length === 0 && (
        <Text color="text.muted" flex="1" display="flex" alignItems="center" justifyContent="center">
          No messages yet
        </Text>
      )}
      {claude ? (
        <DesignChat
          session={session}
          chat={chat}
          refetch={refetch}
          actions={{ address: "" }}
          questions={false}
          onQuestions={noop}
          options={AGENT_OPTIONS}
        />
      ) : (
        <>
          <NoChat session={session} fetchScreen={fetchScreen} />
          <Box flex="1" />
        </>
      )}
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
