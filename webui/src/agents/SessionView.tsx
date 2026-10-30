import { Box, Link, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { useSessionChat, type FetchChat } from "../design/chat";
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
import { TIERS } from "./NewSessionForm";
import { stateLabel, statusMark } from "./rules";
import { StatusMark } from "./SessionGlyphs";
import { SessionSwitcher } from "./SessionSwitcher";

function DesignLink({ map, ml }: { map: string; ml?: string }) {
  return (
    <Link fontSize="sm" ml={ml} color="tone.running.fg" href={designHref(map)}>
      Open design page →
    </Link>
  );
}

/** `<model> · <effort>`; a tier without a model shows `<Tier> · CLI default` until a reply reports one. Empty when nothing is known. */
function modelLabel(s: SessionInfo): { text: string; tier: string } | undefined {
  const tier = TIERS.find((t) => t.id === (s.tier || "worker"))?.label ?? "";
  if (!s.model && !s.effort && !s.tier) return undefined;
  const model = s.model || `${tier} · CLI default`;
  return { text: s.effort ? `${model} · ${s.effort}` : model, tier: s.tier ? tier : "" };
}

function ModelLabel({ session }: { session: SessionInfo }) {
  const label = modelLabel(session);
  if (!label) return null;
  return (
    <Text as="span" fontSize="sm" color="text.dim" title={label.tier ? `${label.tier} tier` : undefined}>
      {label.text}
    </Text>
  );
}

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
        {map && <DesignLink map={map} ml="auto" />}
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
      <ModelLabel session={s} />
      {s.harness !== "claude" && <Chip>{s.harness}</Chip>}
      <Text as="span" fontSize="sm" color="text.dim">
        {s.dir}
      </Text>
      {s.ticket && (
        <Text as="span" fontSize="sm" color="text.dim">
          {s.ticket}
        </Text>
      )}
      {map && <DesignLink map={map} ml="auto" />}
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
        <Button emphasis="secondary" onClick={onCancel}>
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
  const claude = session.harness === "claude";
  const { kind, locked, unlock } = useComposerUnlock(session);
  const [confirming, setConfirming] = useState(false);
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
        chat && <AgentChat session={session} chat={chat} refetch={refetch} />
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
        connected={connected && claude && !locked}
        lastMessage={chat?.messages.at(-1)?.id}
        onStale={refetch}
        reason={locked ? undefined : sendReason(session, connected)}
      />
    </Box>
  );
}
