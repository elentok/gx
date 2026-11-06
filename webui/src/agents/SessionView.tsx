import { Box, Link, Text } from "@chakra-ui/react";
import { useEffect, useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { hasChat, useSessionChat, type FetchChat } from "../design/chat";
import { FailureBanner, needsTranscript, visibleFailure } from "../design/FailureBanner";
import { ScreenModal } from "../design/ScreenModal";
import type { FetchScreen } from "../design/screen";
import { designOf, promptable } from "../design/view";
import type { SessionInfo } from "../generated/server";
import { AGENTS_HREF, designHref } from "../hrefs";
import { useWide } from "../useWide";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { Modal } from "../ui/modal/Modal";
import { AgentChat } from "./AgentChat";
import { AgentComposer } from "./AgentComposer";
import { PromoteButton } from "./PromoteButton";
import { stateLabel, statusMark } from "./rules";
import { AgentLine } from "../design/AgentLine";
import { StatusMark } from "./SessionGlyphs";
import { SessionActions } from "./SessionActions";
import { SessionSwitcher } from "./SessionSwitcher";

function DesignLink({ map, ml }: { map: string; ml?: string }) {
  return (
    <Link fontSize="sm" ml={ml} color="tone.running.fg" href={designHref(map)}>
      Open design page →
    </Link>
  );
}

function SessionHeader({
  session: s,
  sessions,
  onNew,
  promote,
}: {
  session: SessionInfo;
  sessions: SessionInfo[];
  onNew: () => void;
  /** The Promote to Design button; absent for a gx-driven agent. */
  promote?: ReactNode;
}) {
  const navigate = useNavigate();
  const actions = <SessionActions session={s} onRemoved={() => navigate("/agents")} />;
  const { tone } = statusMark(s);
  const map = designOf(s);
  const wide = useWide();
  if (!wide) {
    return (
      <Box as="header" display="flex" alignItems="center" flexWrap="wrap" gap="2" mb="3" pb="2" borderBottomWidth="1px" borderColor="border">
        <Link href={AGENTS_HREF} aria-label="All sessions" fontSize="2xl" lineHeight="1" px="2" color="text.muted">
          ‹
        </Link>
        <StatusMark session={s} />
        <Box display="flex" flexDirection="column" minW="0">
          <SessionSwitcher name={s.name || s.id} sessions={sessions} current={s.id} onNew={onNew} />
          <Text as="span" fontSize="sm" color="text.dim">
            {s.project} · {stateLabel(s)}
          </Text>
          <AgentLine session={s} wide={false} />
        </Box>
        {map && <DesignLink map={map} ml="auto" />}
        <Box ml={map ? undefined : "auto"}>{actions}</Box>
        {promote && <Box flexBasis="100%">{promote}</Box>}
      </Box>
    );
  }
  return (
    <Box as="header" display="flex" alignItems="center" flexWrap="wrap" gap="2" mb="3">
      <StatusMark session={s} />
      <SessionSwitcher name={s.name || s.id} sessions={sessions} current={s.id} onNew={onNew} fontSize="26px" />
      <Chip tone={tone} fontSize="12.5px" outlined={false}>
        {stateLabel(s)}
      </Chip>
      <AgentLine session={s} wide />
      <Text as="span" fontSize="sm" color="text.dim">
        {s.dir}
      </Text>
      {s.ticket && (
        <Text as="span" fontSize="sm" color="text.dim">
          {s.ticket}
        </Text>
      )}
      {map && <DesignLink map={map} ml="auto" />}
      {promote}
      <Box ml={map ? undefined : "auto"}>{actions}</Box>
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
          Chat works for Claude and Codex sessions only.
        </Text>
        <Button emphasis="secondary" onClick={() => setScreen(true)}>
          Show screen
        </Button>
        {screen && <ScreenModal target={session.id} fetcher={fetchScreen} keys={!session.queue} onClose={() => setScreen(false)} />}
      </Box>
    </>
  );
}

/** Why Send is off, first match wins; nothing when the composer may send. */
function sendReason(s: SessionInfo, connected: boolean): string | undefined {
  const state = s.status.state;
  if (!connected) return "Offline. Send turns on when gx reconnects.";
  if (!hasChat(s.harness)) return "Sending works for Claude and Codex sessions only";
  if (state === "exited") return "Exited. Restore it in gx, Agents tab.";
  if (!promptable(s)) {
    return `${stateLabel(s)}. Send turns on when the session is idle.`;
  }
}

/** Which kind of gx-driven agent this is; undefined for a plain session. */
function drivenKind(s: SessionInfo): "Queue" | "Design" | undefined {
  if (s.queue) return "Queue";
  if (s.design) return "Design";
}

/** The composer stays off for a gx-driven agent until a person confirms; selecting another session resets it. */
function useComposerUnlock(session: SessionInfo) {
  const [unlockedId, setUnlockedId] = useState<string>();
  useEffect(() => setUnlockedId(undefined), [session.id]);
  const kind = drivenKind(session);
  return { kind, locked: kind !== undefined && unlockedId !== session.id, unlock: () => setUnlockedId(session.id) };
}

function UnlockPrompt({ kind, onConfirm, onCancel }: { kind: string; onConfirm: () => void; onCancel: () => void }) {
  return (
    <Modal label={`Turn on composer for ${kind} agent`} onClose={onCancel} maxW="sm">
      <Text>gx is driving this agent. Turn on the composer for this {kind} agent anyway?</Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={onCancel} autoFocus>
          Cancel
        </Button>
        <Button onClick={onConfirm}>Turn on composer</Button>
      </Box>
    </Modal>
  );
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
  const chatty = hasChat(session.harness);
  const { kind, locked, unlock } = useComposerUnlock(session);
  const [confirming, setConfirming] = useState(false);
  const { detail: chat, refetch } = useSessionChat(chatty ? session.id : undefined, conn, fetchChat);
  // Prompts the server took that the transcript has not shown yet; each leaves once the chat has one more copy of its text.
  // Each sits right after the message that was last when it was sent, so replies that stream in land below it.
  const [echoed, setEchoed] = useState<(EchoPrompt & { sessionId: string; seen: number; after?: string })[]>([]);
  const waiting = echoed.filter((e) => e.sessionId === session.id && echoMatches(chat?.messages ?? [], e) <= e.seen);
  const shown = chat && waiting.length > 0 ? { ...chat, messages: withEchoes(chat.messages, waiting) } : chat;
  return (
    <Box display="flex" flexDirection="column" height="100%" minH="0">
      <SessionHeader
        session={session}
        sessions={sessions}
        onNew={onNew}
        promote={
          kind === undefined && (
            <PromoteButton
              session={session}
              lastMessage={chat?.messages.at(-1)?.id}
              hasMessages={(chat?.messages.length ?? 0) > 0}
              connected={connected && chatty}
              reason={sendReason(session, connected)}
              onStale={refetch}
            />
          )
        }
      />
      {chatty && chat && chat.messages.length === 0 && !chat.archived && !chat.unreadable && (
        <Text color="text.muted" flex="1" display="flex" alignItems="center" justifyContent="center">
          No messages yet
        </Text>
      )}
      {chatty ? (
        shown && <AgentChat session={session} chat={shown} refetch={refetch} />
      ) : (
        <>
          <NoChat session={session} fetchScreen={fetchScreen} />
          <Box flex="1" />
        </>
      )}
      {locked && (
        <Box display="flex" alignItems="center" gap="2" flexWrap="wrap">
          <Text fontSize="sm" color="text.dim">
            gx is driving this agent, so the composer is off.
          </Text>
          <Button emphasis="secondary" onClick={() => setConfirming(true)}>
            Turn on composer
          </Button>
        </Box>
      )}
      {confirming && kind && (
        <UnlockPrompt
          kind={kind}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            unlock();
            setConfirming(false);
          }}
        />
      )}
      <AgentComposer
        session={session}
        connected={connected && chatty && !locked}
        lastMessage={chat?.messages.at(-1)?.id}
        onStale={refetch}
        onSent={(sent, images) => {
          const echo = { text: stripChips(sent), images };
          setEchoed((all) => [...all, { ...echo, sessionId: session.id, seen: echoMatches(chat?.messages ?? [], echo), after: chat?.messages.at(-1)?.id }]);
        }}
        reason={locked ? undefined : sendReason(session, connected)}
      />
    </Box>
  );
}

export type EchoPrompt = { text: string; images: string[] };

// The transcript text carries "[Image #N]" chips the sent text does not, and stores it trimmed, so both sides drop the chips and the edges.
const stripChips = (text: string) => text.replace(/\[Image #\d+\]/g, "").trim();

// How many transcript messages are this prompt. A slash command shows up as a "command" message, so it counts too. An images-only prompt has
// no text to match, so it matches the images-only prompts with the same image count.
export function echoMatches(messages: { kind: string; text?: string; images?: unknown[] }[], e: EchoPrompt): number {
  return messages.filter((m) => {
    if (m.kind !== "prompt" && m.kind !== "command") return false;
    const text = stripChips(m.text ?? "");
    return e.text === "" && e.images.length > 0 ? text === "" && (m.images?.length ?? 0) === e.images.length : text === e.text;
  }).length;
}

// Puts each echo after its anchor message, in send order. An echo sent into an empty chat goes first; one whose anchor is gone goes last.
export function withEchoes<M extends { id: string }>(messages: M[], echoes: (EchoPrompt & { after?: string })[]): M[] {
  const echo = (e: EchoPrompt, i: number) =>
    ({
      id: `echo-${i}`,
      role: "person",
      kind: "prompt",
      text: e.text,
      at: "",
      ...(e.images.length > 0 && { images: e.images.map((id) => ({ id })) }),
    }) as unknown as M;
  const known = new Set(messages.map((m) => m.id));
  const out: M[] = [];
  echoes.forEach((e, i) => e.after === undefined && out.push(echo(e, i)));
  for (const m of messages) {
    out.push(m);
    echoes.forEach((e, i) => e.after === m.id && out.push(echo(e, i)));
  }
  echoes.forEach((e, i) => e.after !== undefined && !known.has(e.after) && out.push(echo(e, i)));
  return out;
}
