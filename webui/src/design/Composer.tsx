import { Box, Spinner, Text, Textarea } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import type {
  SessionInfo,
  SessionNext,
  SessionPromptRequest,
  SessionRoundCard,
} from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
import { useWrite } from "../useWrite";
import { AutoAdvanceBar, type RunCall } from "./AutoAdvanceBar";
import {
  postAutoAdvance,
  postCancelAutoAdvance,
  postInterruptSession,
  stateToken,
  type CancelAutoAdvance,
  type InterruptSession,
  type SetAutoAdvance,
} from "./open";
import { postPrompt, type PostPrompt } from "./prompt";
import { roundPrompt, unansweredOf, type Answers } from "./round";
import { SendPopover } from "./SendPopover";
import { useWide } from "./useWide";
import { promptable } from "./view";

// Under the chat, one line: the auto-advance switch, then Note and the main action. Nothing sends unless the session is Idle.
export function Composer({
  session,
  lastMessage,
  next,
  nextTitle,
  connected,
  post = postPrompt,
  interrupt = postInterruptSession,
  setAutoAdvance = postAutoAdvance,
  cancelAutoAdvance = postCancelAutoAdvance,
  onStale,
  round = [],
  answers = {},
  initialText = "",
  onText,
  onRoundSent,
  onReview,
}: {
  session: SessionInfo;
  /** The id of the last chat message the page saw. */
  lastMessage?: string;
  next?: SessionNext;
  /** Shown under Continue: the map's next ticket title, or the step label. */
  nextTitle?: string;
  connected: boolean;
  post?: PostPrompt;
  interrupt?: InterruptSession;
  setAutoAdvance?: SetAutoAdvance;
  cancelAutoAdvance?: CancelAutoAdvance;
  /** The conversation moved under the person: refetch the chat, keep the draft. */
  onStale?: () => void;
  /** The open Grilling round: Send answers all of it in one prompt. */
  round?: SessionRoundCard[];
  answers?: Answers;
  /** The notes box starts with this text, and `onText` hears every change, so the caller can keep a draft. */
  initialText?: string;
  onText?: (text: string) => void;
  /** The round went out. */
  onRoundSent?: () => void;
  /** A row of the unanswered popover: open the sheet at that question. */
  onReview?: (index: number) => void;
}) {
  const wide = useWide();
  const [open, setOpen] = useState(false);
  const [unanswered, setUnanswered] = useState<SessionRoundCard[]>([]);
  const [text, setTextState] = useState(initialText);
  const setText = (t: string) => {
    setTextState(t);
    onText?.(t);
  };
  const { run: write, pending } = useWrite();
  const enabled = connected && promptable(session) && !pending;

  const send = (body: Partial<SessionPromptRequest>, onSent?: () => void) => {
    if (!enabled) return;
    return write(
      async () => {
        const res = await post({
          id: session.id,
          text: "",
          last_message: lastMessage,
          ...body,
        });
        if (res.refused && res.reason === ReasonStale) onStale?.();
        return res;
      },
      { onDone: onSent },
    );
  };

  // A Working session takes no prompt: Interrupt ends its turn and keeps the session.
  const working = session.status.state === "working" && !promptable(session);
  const run: RunCall = (call) => (connected ? write(call) : Promise.resolve());
  const onInterrupt = () =>
    run(() => interrupt({ id: session.id, state: stateToken(session) }));

  const inRound = round.length > 0;
  const sendRound = () => {
    setUnanswered([]);
    return send({ text: roundPrompt(round, answers, text) }, () => {
      setText("");
      onRoundSent?.();
    });
  };
  const onSend = () => {
    if (!inRound) return send({ text }, () => setText(""));
    if (unanswered.length > 0) return setUnanswered([]);
    const missing = unansweredOf(round, answers);
    if (missing.length > 0) setUnanswered(missing);
    else sendRound();
  };

  const canContinue = !!next?.action.action?.length;
  // Interrupt while the agent works; Continue after a run-end line unless a note or round is waiting to go.
  const mainButton = working ? (
    <Button
      tone="deadlocked"
      disabled={!connected || pending}
      onClick={onInterrupt}
    >
      {pending && <Spinner size="sm" />}
      Interrupt
    </Button>
  ) : canContinue && !inRound && text.trim() === "" ? (
    <Button
      aria-label={nextTitle ? `Continue ${nextTitle}` : "Continue"}
      disabled={!enabled}
      onClick={() => send({ continue: true })}
    >
      {pending && <Spinner size="sm" />}
      Continue
    </Button>
  ) : (
    withPopover(
      <Button
        disabled={!enabled || (!inRound && text.trim() === "")}
        onClick={onSend}
      >
        {pending && <Spinner size="sm" />}
        Send
      </Button>,
    )
  );
  // With gaps, Send's click opens the popover instead of sending; a click while it is open closes it.
  function withPopover(send: ReactNode) {
    return (
      <SendPopover
        unanswered={unanswered}
        total={round.length}
        sendDisabled={!enabled}
        onSendAnyway={sendRound}
        onAnswer={(c) =>
          onReview?.(round.findIndex((r) => r.label === c.label))
        }
        onClose={() => setUnanswered([])}
      >
        {send}
      </SendPopover>
    );
  }

  const autoAdvanceBar = (
    <AutoAdvanceBar
      session={session}
      disabled={!connected || pending}
      run={run}
      setAutoAdvance={setAutoAdvance}
      cancelAutoAdvance={cancelAutoAdvance}
    />
  );

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface"
      borderTopWidth="1px"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      role="form"
      aria-label="Composer"
    >
      {open && (
        <Textarea
          aria-label="Message"
          borderRadius="12px"
          value={text}
          disabled={!enabled}
          onChange={(e) => setText(e.target.value)}
          autoFocus
        />
      )}
      {/* One line: the switch on the left, the main action on the right. */}
      <Box display="flex" gap="2" alignItems="center">
        {autoAdvanceBar}
        <Box flex="1" />
        {canContinue && !working && (
          <Text as="span" fontSize="xs" color="text.dim" truncate minW="0">
            {nextTitle}
          </Text>
        )}
        <Button
          emphasis="secondary"
          aria-label="Note"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          ✎{wide && " Note"}
        </Button>
        {mainButton}
      </Box>
    </Box>
  );
}
