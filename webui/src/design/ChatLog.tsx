import { Box, Spinner as ChakraSpinner, Text } from "@chakra-ui/react";
import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import type {
  SessionBackgroundTask,
  SessionChatMessage,
  SessionChatResponse,
  SessionInfo,
} from "../generated/server";
import { Markdown } from "../markdown/Markdown";
import { useSessionChat, type FetchChat } from "./chat";
import { Composer } from "./Composer";
import {
  FailureBanner,
  GxStopped,
  needsTranscript,
  replacesComposer,
  visibleFailure,
  type FailureActions,
} from "./FailureBanner";
import type { PostPrompt } from "./prompt";
import { QuestionSheet } from "./QuestionSheet";
import { RoundFocus } from "./RoundFocus";
import { useRoundDraft } from "./roundDraft";

const KIND_NAMES: Record<string, string> = {
  shell: "shell",
  monitor: "Monitor",
  agent: "subagent",
};

function Entry({ m }: { m: SessionChatMessage }) {
  if (m.kind === "compacted") {
    return (
      <Text
        role="separator"
        textAlign="center"
        fontSize="xs"
        color="text.dim"
        borderBlockStart="1px solid"
        borderColor="border"
        pt="1"
      >
        Context compacted
      </Text>
    );
  }
  if (m.kind === "tools") return null;
  if (m.kind === "prompt") {
    return (
      <Box
        alignSelf="flex-end"
        maxW="85%"
        bg="surface.raised"
        borderRadius="lg"
        px="3"
        py="2"
        fontSize="sm"
        whiteSpace="pre-wrap"
      >
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
    <Box
      role="list"
      aria-label="Running in background"
      display="flex"
      flexWrap="wrap"
      gap="2"
      fontSize="xs"
      color="text.muted"
    >
      {tasks.map((t, i) => (
        <Box
          key={i}
          role="listitem"
          border="1px solid"
          borderColor="border"
          borderRadius="md"
          px="2"
          py="0.5"
        >
          {KIND_NAMES[t.kind] ?? t.kind}: {t.label}
        </Box>
      ))}
    </Box>
  );
}

export interface WorkingStep {
  step?: string;
  title?: string;
}

// Inline, not the page-centered Spinner: this one sits at the bottom of the chat.
function WorkingRow({ step, title }: WorkingStep) {
  return (
    <Box
      role="status"
      display="flex"
      alignItems="center"
      gap="2"
      fontSize="sm"
      color="text.muted"
    >
      <ChakraSpinner size="sm" />
      <Text as="span">
        Working
        {step && `: ${step}`}
        {title && ` · ${title}`}
      </Text>
    </Box>
  );
}

// The sheet and the composer share the round's answers. The caller keys it by round, so a new round starts blank
// while a refetch of the same round keeps every answer. The draft also survives a reload (see useRoundDraft).
function ChatTail({
  session,
  roundKey,
  chat,
  connected,
  post,
  onStale,
  done,
  nextTitle,
  actions,
}: {
  session: SessionInfo;
  roundKey: string;
  chat: SessionChatResponse;
  connected: boolean;
  post?: PostPrompt;
  onStale: () => void;
  done?: boolean;
  nextTitle?: string;
  actions: FailureActions;
}) {
  const { draft, setAnswers, setNotes, setIndex, clear } = useRoundDraft(
    session.id,
    roundKey,
  );
  const { answers } = draft;
  const [focus, setFocus] = useState<{ index: number }>();
  // Full screen, the composer renders into the overlay's footer; the docked sheet stays mounted, hidden, so it keeps its place.
  const [full, setFull] = useState(false);
  const [footer, setFooter] = useState<HTMLDivElement | null>(null);
  const close = useCallback(() => setFull(false), []);
  const failure = visibleFailure(session.failure, done);
  const round = chat.round ?? [];
  return (
    // The bottom area: at most half the screen, so only the question body scrolls past that.
    <Box
      flex="none"
      maxH="50dvh"
      display="flex"
      flexDirection="column"
      minH="0"
      gap="3"
    >
      {round.length > 0 && (
        <Box
          hidden={full}
          display={full ? "none" : "flex"}
          flexDirection="column"
          minH="0"
        >
          <QuestionSheet
            round={round}
            answers={answers}
            onAnswers={setAnswers}
            focus={focus}
            initialIndex={draft.index}
            onIndex={setIndex}
          />
        </Box>
      )}
      {full && round.length > 0 && (
        <RoundFocus
          round={round}
          answers={answers}
          onAnswers={setAnswers}
          focus={focus}
          onClose={close}
          footer={setFooter}
        />
      )}
      {failure && (
        <FailureBanner
          failure={failure}
          session={session}
          lastMessage={chat.messages.at(-1)?.id}
          post={post}
          onStale={onStale}
          {...actions}
        />
      )}
      {!failure &&
        session.actionable.gx_raised &&
        session.actionable.reason && (
          <GxStopped reason={session.actionable.reason} />
        )}
      {!done && !replacesComposer(failure) && (
        <Composer
          session={session}
          lastMessage={chat.messages.at(-1)?.id}
          next={chat.next}
          nextTitle={nextTitle}
          connected={connected}
          post={post}
          onStale={onStale}
          round={round}
          answers={answers}
          initialText={draft.notes}
          onText={setNotes}
          onRoundSent={clear}
          onReview={(index) => setFocus({ index })}
          fullScreen={full ? footer : undefined}
          onFullScreen={setFull}
        />
      )}
    </Box>
  );
}

// The messages scroll alone. They open at the newest one and follow new ones until the reader scrolls up.
function ChatScroller({
  children,
  signal,
}: {
  children: ReactNode;
  /** Changes whenever the content grows. */
  signal: unknown;
}) {
  const el = useRef<HTMLDivElement>(null);
  const stuck = useRef(true);
  useLayoutEffect(() => {
    if (el.current && stuck.current) el.current.scrollTop = el.current.scrollHeight;
  }, [signal]);
  return (
    <Box
      ref={el}
      flex="1"
      minH="0"
      overflowY="auto"
      display="flex"
      flexDirection="column"
      gap="3"
      onScroll={(e) => {
        const t = e.currentTarget;
        stuck.current = t.scrollHeight - t.scrollTop - t.clientHeight < 80;
      }}
    >
      {children}
    </Box>
  );
}

// The chat of a Claude session. Other harnesses have no chat, so DesignChat never mounts this for them.
function ChatLog({
  session,
  fetchChat,
  post,
  done,
  nextTitle,
  working: step,
  actions,
}: {
  session: SessionInfo;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  done?: boolean;
  nextTitle?: string;
  working?: WorkingStep;
  actions: FailureActions;
}) {
  const conn = useConnectionContext();
  const { detail: chat, refetch } = useSessionChat(session.id, conn, fetchChat);
  const connected = useConnectionState(conn).connected;
  if (!chat) return null;
  const roundKey = (chat.round ?? [])
    .map((c) => `${c.label}:${c.title}`)
    .join("|");
  const busy =
    session.status.state === "working" && !session.status.background;
  return (
    <Box display="flex" flexDirection="column" gap="3" flex="1" minH="0">
      <ChatScroller
        signal={`${chat.messages.length}|${chat.background.length}|${busy}`}
      >
        {chat.truncated && (
          <Text fontSize="xs" color="text.dim" textAlign="center">
            Earlier messages hidden
          </Text>
        )}
        {chat.messages.map((m, i) => (
          <Entry key={m.id || i} m={m} />
        ))}
        {chat.background.length > 0 && (
          <BackgroundStrip tasks={chat.background} />
        )}
        {busy && step && <WorkingRow {...step} />}
      </ChatScroller>
      <ChatTail
        key={roundKey}
        session={session}
        roundKey={roundKey}
        chat={chat}
        connected={connected}
        post={post}
        onStale={refetch}
        done={done}
        nextTitle={nextTitle}
        actions={actions}
      />
    </Box>
  );
}

/** The center column of a Design: its session's chat, or an empty chat when it has none. */
export function DesignChat({
  session,
  fetchChat,
  post,
  done,
  nextTitle,
  working,
  actions = {},
}: {
  session?: SessionInfo;
  fetchChat?: FetchChat;
  post?: PostPrompt;
  /** The Design's Progress is Done: no composer, no `ended` banner. */
  done?: boolean;
  /** The subtitle under Continue: the next ticket's title, or the step label. */
  nextTitle?: string;
  /** The step the busy row names, and the ticket during Resolve tickets. */
  working?: WorkingStep;
  actions?: FailureActions;
}) {
  if (!session) return null;
  if (session.harness !== "claude") {
    const failure = visibleFailure(session.failure, done);
    return (
      <>
        {failure && !needsTranscript(failure) && (
          <FailureBanner failure={failure} session={session} {...actions} />
        )}
        <Text color="text.muted" fontSize="sm">
          Chat works for Claude sessions only. Open it in gx, Agents tab.
        </Text>
      </>
    );
  }
  return (
    <ChatLog
      key={session.id}
      session={session}
      fetchChat={fetchChat}
      post={post}
      done={done}
      nextTitle={nextTitle}
      working={working}
      actions={actions}
    />
  );
}
