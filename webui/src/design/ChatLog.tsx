import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useState, type ReactNode } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import type { SessionChatResponse, SessionInfo } from "../generated/server";
import { roundKeyOf } from "./chat";
import { ChatMessages, WorkingRow, type WorkingStep } from "./ChatCore";
import type { AutoMenuState } from "./AutoMenu";
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
import { isAnswered } from "./round";
import { type RoundDraft, useRoundDraft } from "./roundDraft";

// The sheet and the composer share the round's answers (see useRoundDraft: they survive a reload).
function ChatTail({
  session,
  chat,
  connected,
  post,
  onStale,
  done,
  nextTitle,
  actions,
  questions,
  onQuestions,
  roundDraft,
  autoMenu,
}: {
  session: SessionInfo;
  chat: SessionChatResponse;
  connected: boolean;
  post?: PostPrompt;
  onStale: () => void;
  done?: boolean;
  nextTitle?: string;
  actions: FailureActions;
  questions: boolean;
  onQuestions: (on: boolean) => void;
  roundDraft: RoundDraft;
  autoMenu: AutoMenuState;
}) {
  const { draft, setAnswers, setIndex, clear } = roundDraft;
  const { answers } = draft;
  const [focus, setFocus] = useState<{ index: number }>();
  const failure = visibleFailure(session.failure, done);
  const round = chat.round ?? [];
  return (
    // The bottom area: at most half the screen, so only the question body scrolls past that.
    // In the Questions view the cap is gone and the sheet takes the rest of the column.
    <Box
      flex={questions ? "1" : "none"}
      maxH={questions ? undefined : "50dvh"}
      display="flex"
      flexDirection="column"
      minH="0"
      gap="3"
    >
      {round.length > 0 && (
        <QuestionSheet
          round={round}
          answers={answers}
          onAnswers={setAnswers}
          focus={focus}
          initialIndex={draft.index}
          onIndex={setIndex}
          questions={questions}
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
          onRoundSent={() => {
            clear();
            onQuestions(false);
          }}
          onReview={(index) => setFocus({ index })}
          autoMenu={autoMenu}
        />
      )}
    </Box>
  );
}

// Every dialog and popover (Send, jump menu, screen, steps) is one of these; Escape closes the top one first.
const LAYERS = '[role="dialog"],[role="alertdialog"],[role="menu"]';

function ViewToggle({
  answered,
  total,
  questions,
  onQuestions,
}: {
  answered: number;
  total: number;
  questions: boolean;
  onQuestions: (on: boolean) => void;
}) {
  return (
    <Box
      role="group"
      aria-label="View"
      display="flex"
      gap="3px"
      p="3px"
      flex="none"
      bg="surface.raised"
      borderRadius="10px"
    >
      <SegButton on={questions} onClick={() => onQuestions(true)}>
        Questions ({answered} of {total})
      </SegButton>
      <SegButton on={!questions} onClick={() => onQuestions(false)}>
        Chat
      </SegButton>
    </Box>
  );
}

function SegButton({
  on,
  onClick,
  children,
}: {
  on: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <chakra.button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      flex="1"
      minH="40px"
      whiteSpace="nowrap"
      px="3"
      borderRadius="8px"
      fontWeight="semibold"
      bg={on ? "surface" : "transparent"}
      color={on ? "text" : "text.muted"}
      boxShadow={on ? "0 0 0 1px {colors.border}" : "none"}
      fontSize="15px"
      cursor="pointer"
    >
      {children}
    </chakra.button>
  );
}

// Everything that depends on the round. The caller keys it by round, so a new round starts blank (and in the
// Questions view) while a refetch of the same round keeps every answer.
function ChatBody({
  session,
  roundKey,
  chat,
  connected,
  post,
  onStale,
  done,
  nextTitle,
  working: step,
  actions,
  questions,
  onQuestions,
  autoMenu,
}: {
  session: SessionInfo;
  roundKey: string;
  chat: SessionChatResponse;
  connected: boolean;
  post?: PostPrompt;
  onStale: () => void;
  done?: boolean;
  nextTitle?: string;
  working?: WorkingStep;
  actions: FailureActions;
  questions: boolean;
  onQuestions: (on: boolean) => void;
  autoMenu: AutoMenuState;
}) {
  const roundDraft = useRoundDraft(session.id, roundKey);
  const round = chat.round ?? [];
  const hasRound = round.length > 0;
  useEffect(() => {
    if (!questions) return;
    // Capture phase: judge the layers as they were before this same Escape closes one of them.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || document.querySelector(LAYERS)) return;
      onQuestions(false);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [questions, onQuestions]);
  const busy = session.status.state === "working" && !session.status.background;
  return (
    <Box display="flex" flexDirection="column" gap="3" flex="1" minH="0">
      {hasRound && (
        <ViewToggle
          answered={
            round.filter((c) => isAnswered(roundDraft.draft.answers[c.label]))
              .length
          }
          total={round.length}
          questions={questions}
          onQuestions={onQuestions}
        />
      )}
      {!questions && (
        <ChatMessages
          chat={chat}
          busy={busy}
          working={step && <WorkingRow {...step} />}
        />
      )}
      <ChatTail
        session={session}
        chat={chat}
        connected={connected}
        post={post}
        onStale={onStale}
        done={done}
        nextTitle={nextTitle}
        actions={actions}
        questions={questions}
        onQuestions={onQuestions}
        roundDraft={roundDraft}
        autoMenu={autoMenu}
      />
    </Box>
  );
}

// The chat of a Claude session. Other harnesses have no chat, so DesignChat never mounts this for them.
function ChatLog({
  session,
  chat,
  refetch,
  ...rest
}: {
  session: SessionInfo;
  chat: SessionChatResponse;
  refetch: () => void;
  post?: PostPrompt;
  done?: boolean;
  nextTitle?: string;
  working?: WorkingStep;
  actions: FailureActions;
  questions: boolean;
  onQuestions: (on: boolean) => void;
}) {
  const conn = useConnectionContext();
  const connected = useConnectionState(conn).connected;
  // Lives above the keyed ChatBody: a round landing remounts it and must not close the Auto popover.
  const [autoMenuOpen, setAutoMenuOpen] = useState(false);
  const roundKey = roundKeyOf(chat);
  return (
    <ChatBody
      key={roundKey}
      autoMenu={{ open: autoMenuOpen, setOpen: setAutoMenuOpen }}
      session={session}
      roundKey={roundKey}
      chat={chat}
      connected={connected}
      onStale={refetch}
      {...rest}
    />
  );
}

/** The center column of a Design: its session's chat, or an empty chat when it has none. */
export function DesignChat({
  session,
  chat,
  refetch,
  post,
  done,
  nextTitle,
  working,
  actions,
  questions,
  onQuestions,
}: {
  session?: SessionInfo;
  /** The Claude session's chat, fetched by the page; null while it loads. */
  chat?: SessionChatResponse | null;
  refetch: () => void;
  post?: PostPrompt;
  /** The Design's Progress is Done: no composer, no `ended` banner. */
  done?: boolean;
  /** The subtitle under Continue: the next ticket's title, or the step label. */
  nextTitle?: string;
  /** The step the busy row names, and the ticket during Resolve tickets. */
  working?: WorkingStep;
  actions: FailureActions;
  /** The Questions view: only the questions sheet, the failure notices and the composer. The page derives it. */
  questions: boolean;
  onQuestions: (on: boolean) => void;
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
  if (!chat) return null;
  return (
    <ChatLog
      key={session.id}
      session={session}
      chat={chat}
      refetch={refetch}
      post={post}
      done={done}
      nextTitle={nextTitle}
      working={working}
      actions={actions}
      questions={questions}
      onQuestions={onQuestions}
    />
  );
}
