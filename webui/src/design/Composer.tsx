import { Box, Text, Textarea } from "@chakra-ui/react";
import { useState } from "react";
import { createPortal } from "react-dom";
import type {
  SessionInfo,
  SessionNext,
  SessionPromptRequest,
  SessionRoundCard,
} from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
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
import {
  isAnswered,
  nameOf,
  roundPrompt,
  unansweredOf,
  type Answers,
} from "./round";
import { useWide } from "./useWide";
import { promptable } from "./view";

// Under the chat, one line: the auto-advance switch, then Note, Focus and the main action. Nothing sends unless the session is Idle.
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
  fullScreen,
  onFullScreen,
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
  /** "Answer them" in the unanswered confirm: open the sheet at that question. */
  onReview?: (index: number) => void;
  /** The round is full screen: the composer renders into this element, its note always open. */
  fullScreen?: HTMLElement | null;
  /** Opens or closes the full-screen round; a sent round closes it. */
  onFullScreen?: (on: boolean) => void;
}) {
  const wide = useWide();
  const [open, setOpen] = useState(false);
  const [unanswered, setUnanswered] = useState<SessionRoundCard[]>([]);
  const [text, setTextState] = useState(initialText);
  const setText = (t: string) => {
    setTextState(t);
    onText?.(t);
  };
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const enabled = connected && promptable(session) && !pending;

  const send = async (
    body: Partial<SessionPromptRequest>,
    onSent?: () => void,
  ) => {
    if (!enabled) return;
    setPending(true);
    setError("");
    try {
      const res = await post({
        id: session.id,
        text: "",
        last_message: lastMessage,
        ...body,
      });
      if (res.refused) {
        setError(res.message ?? "The message was refused.");
        if (res.reason === ReasonStale) onStale?.();
      } else {
        onSent?.();
      }
    } catch {
      setError("Could not send. Try again.");
    } finally {
      setPending(false);
    }
  };

  // A Working session takes no prompt: Interrupt ends its turn and keeps the session.
  const working = session.status.state === "working" && !promptable(session);
  // The calls that are not a prompt: one pending flag, one error line; a refusal shows its message, a throw shows `failure`.
  const run: RunCall = async (call, failure) => {
    if (!connected || pending) return;
    setPending(true);
    setError("");
    try {
      const res = await call();
      if (res.refused) setError(res.message ?? failure);
    } catch {
      setError(failure);
    } finally {
      setPending(false);
    }
  };
  const onInterrupt = () =>
    run(
      () => interrupt({ id: session.id, state: stateToken(session) }),
      "Could not interrupt. Try again.",
    );

  const inRound = round.length > 0;
  const sendRound = () => {
    setUnanswered([]);
    return send({ text: roundPrompt(round, answers, text) }, () => {
      setText("");
      onRoundSent?.();
      onFullScreen?.(false);
    });
  };
  const onSend = () => {
    if (!inRound) return send({ text }, () => setText(""));
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
      Interrupt
    </Button>
  ) : canContinue && !inRound && text.trim() === "" ? (
    <Button
      aria-label={nextTitle ? `Continue ${nextTitle}` : "Continue"}
      disabled={!enabled}
      onClick={() => send({ continue: true })}
    >
      Continue
    </Button>
  ) : (
    <Button
      disabled={!enabled || (!inRound && text.trim() === "")}
      onClick={onSend}
    >
      Send
    </Button>
  );
  const focusButton = inRound && onFullScreen && (
    <Button
      emphasis="secondary"
      aria-label="Focus"
      onClick={() => onFullScreen(true)}
    >
      ⤢{wide && " Focus"}
    </Button>
  );
  const notices = (
    <>
      {error && <Text role="alert">{error}</Text>}
      {unanswered.length > 0 && (
        <Box
          role="alertdialog"
          aria-label="Unanswered questions"
          display="flex"
          flexDirection="column"
          gap="2"
        >
          <Text>
            {unanswered.length}{" "}
            {unanswered.length === 1 ? "question is" : "questions are"}{" "}
            unanswered ({unanswered.map(nameOf).join(", ")}). Send anyway?
          </Text>
          <Box display="flex" gap="2">
            <Button
              emphasis="secondary"
              onClick={() => {
                onReview?.(
                  round.findIndex((c) => c.label === unanswered[0].label),
                );
                setUnanswered([]);
              }}
            >
              Answer them
            </Button>
            <Button disabled={!enabled} onClick={sendRound}>
              Send anyway
            </Button>
          </Box>
        </Box>
      )}
    </>
  );

  const autoAdvanceBar = (
    <AutoAdvanceBar
      session={session}
      disabled={!connected || pending}
      run={run}
      setAutoAdvance={setAutoAdvance}
      cancelAutoAdvance={cancelAutoAdvance}
    />
  );

  if (fullScreen) {
    const done = round.filter((c) => isAnswered(answers[c.label])).length;
    return createPortal(
      <Box
        borderTopWidth="1px"
        bg="surface"
        px="4"
        py="3"
        display="flex"
        flexDirection="column"
        gap="2"
        role="form"
        aria-label="Composer"
      >
        {notices}
        {autoAdvanceBar}
        <Box
          display="flex"
          gap="3"
          flexDirection={{ base: "column", md: "row" }}
        >
          <Textarea
            aria-label="Message"
            flex="1"
            rows={2}
            borderRadius="12px"
            placeholder="Note for the whole round (optional)"
            value={text}
            disabled={!enabled}
            onChange={(e) => setText(e.target.value)}
          />
          <Box
            display="flex"
            flexDirection="column"
            gap="1"
            justifyContent="flex-end"
          >
            <Text fontSize="xs" color="text.dim" textAlign="center">
              {done} of {round.length} answered
            </Text>
            <Button
              disabled={!enabled}
              onClick={onSend}
              size="md"
              justifyContent="center"
              px="6"
            >
              Send
            </Button>
          </Box>
        </Box>
      </Box>,
      fullScreen,
    );
  }

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
      {notices}
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
        {focusButton}
        {mainButton}
      </Box>
    </Box>
  );
}
