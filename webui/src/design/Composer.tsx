import { Box, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { useComposerDraft } from "../drafts";
import type {
  SessionInfo,
  SessionNext,
  SessionRoundCard,
} from "../generated/server";
import { Button } from "../ui/button/Button";
import { AutoAdvanceBar, type RunCall } from "./AutoAdvanceBar";
import type { AutoMenuState } from "./AutoMenu";
import {
  postAutoAdvance,
  postCancelAutoAdvance,
  postInterruptSession,
  stateToken,
  type CancelAutoAdvance,
  type InterruptSession,
  type SetAutoAdvance,
} from "./open";
import { codexSkillHint } from "./chat";
import { ComposerImages } from "./ComposerImages";
import { InterruptPopover } from "./InterruptPopover";
import { useComposerImages } from "./useComposerImages";
import { NoteBox } from "./NoteBox";
import type { PostPrompt } from "./prompt";
import { roundPrompt, unansweredOf, type Answers } from "./round";
import { SendPopover } from "./SendPopover";
import { usePromptSend } from "./usePromptSend";
import { useWide } from "../useWide";
import { promptable } from "./view";

// Under the chat, one line: the Auto menu, then Note and the main action. Nothing sends unless the session is Idle.
export function Composer({
  session,
  lastMessage,
  next,
  nextTitle,
  connected,
  post,
  interrupt = postInterruptSession,
  setAutoAdvance = postAutoAdvance,
  cancelAutoAdvance = postCancelAutoAdvance,
  onStale,
  round = [],
  answers = {},
  onRoundSent,
  onReview,
  autoMenu,
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
  /** The round went out. */
  onRoundSent?: () => void;
  /** A row of the unanswered popover: open the sheet at that question. */
  onReview?: (index: number) => void;
  /** The Auto popover's open state, kept by the caller so a remount of the composer does not close it. */
  autoMenu?: AutoMenuState;
}) {
  const wide = useWide();
  const [gapsOpen, setGapsOpen] = useState(false);
  const sendRef = useRef<HTMLButtonElement>(null);
  const [interruptOpen, setInterruptOpen] = useState(false);
  const interruptRef = useRef<HTMLButtonElement>(null);
  const [text, setText] = useComposerDraft(session.id);
  // Shared text is never hidden behind a closed Note: it opens on load and on every change (another tab).
  const images = useComposerImages(session.id);
  const hasImages = images.images.length > 0;
  const shared = text !== "" || hasImages;
  // A Working session takes no prompt: Interrupt ends its turn and keeps the session.
  const working = session.status.state === "working" && !promptable(session);
  const [open, setOpen] = useState(shared);
  // Adding an image while the agent works must not force the note open.
  useEffect(() => {
    if (shared && !working) setOpen(true);
  }, [shared, working]);
  const hasContent = images.hasContent(text);
  const { send, write, enabled, pending } = usePromptSend({
    session,
    lastMessage,
    connected,
    post,
    onStale,
    toastRefusal: true,
  });

  const run: RunCall = (call) => (connected ? write(call) : Promise.resolve());
  const onInterrupt = () => {
    setInterruptOpen(false);
    return run(() => interrupt({ id: session.id, state: stateToken(session) }));
  };

  const inRound = round.length > 0;
  const sendRound = () => {
    setGapsOpen(false);
    return send({ text: roundPrompt(round, answers, text), ...images.payload }, () => {
      setText("");
      images.clear();
      onRoundSent?.();
    });
  };
  const unanswered = unansweredOf(round, answers);
  // With gaps, Send's click opens the popover instead of sending; a click while it is open closes it.
  const onSendRound = () => {
    if (gapsOpen) setGapsOpen(false);
    else if (images.blocking) return;
    else if (unanswered.length > 0) setGapsOpen(true);
    else sendRound();
  };

  const sendText = () =>
    send({ text, ...images.payload }, () => {
      setText("");
      images.clear();
    });
  // Cmd/Ctrl+Enter runs the main button only when that is Send or Send answers; a phone has no key send.
  const onSubmit = wide
    ? () => {
        if (!enabled) return;
        if (inRound) onSendRound();
        else if (images.canSend(text)) sendText();
      }
    : undefined;

  const canContinue = !!next?.action.action?.length;
  // Interrupt while the agent works; Continue after a run-end line unless a note or round is waiting to go.
  const mainButton = working ? (
    <>
      <Button
        ref={interruptRef}
        emphasis="secondary"
        disabled={!connected || pending}
        aria-expanded={interruptOpen}
        onClick={() => setInterruptOpen(!interruptOpen)}
      >
        {pending && <Spinner size="sm" />}
        Interrupt
      </Button>
      <InterruptPopover
        open={interruptOpen}
        disabled={!connected || pending}
        anchorRef={interruptRef}
        onConfirm={onInterrupt}
        onClose={() => setInterruptOpen(false)}
      />
    </>
  ) : canContinue && !inRound && !hasContent ? (
    <Button
      aria-label={nextTitle ? `Continue ${nextTitle}` : "Continue"}
      disabled={!enabled}
      onClick={() => send({ continue: true })}
    >
      {pending && <Spinner size="sm" />}
      Continue
    </Button>
  ) : inRound ? (
    <>
      <Button ref={sendRef} disabled={!enabled || images.blocking} onClick={onSendRound}>
        {pending && <Spinner size="sm" />}
        Send answers
      </Button>
      <SendPopover
        open={gapsOpen && unanswered.length > 0}
        unanswered={unanswered}
        total={round.length}
        sendDisabled={!enabled || images.blocking}
        anchorRef={sendRef}
        onSendAnyway={sendRound}
        onAnswer={(c) =>
          onReview?.(round.findIndex((r) => r.label === c.label))
        }
        onClose={() => setGapsOpen(false)}
      />
    </>
  ) : (
    <Button disabled={!enabled || !images.canSend(text)} onClick={sendText}>
      {pending && <Spinner size="sm" />}
      Send
    </Button>
  );

  const autoAdvanceBar = autoMenu && (
    <AutoAdvanceBar
      session={session}
      disabled={!connected || pending}
      run={run}
      setAutoAdvance={setAutoAdvance}
      cancelAutoAdvance={cancelAutoAdvance}
      menu={autoMenu}
    />
  );

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface"
      borderTopWidth={wide ? "1px" : "0"}
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      role="form"
      aria-label="Composer"
    >
      <ComposerImages
        state={images}
        send={
          <>
            {autoAdvanceBar}
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
          </>
        }
      >
        {open && (
          <NoteBox
            name="the composer"
            label="Message"
            title={inRound ? "Note on this round" : "Message to the agent"}
            placeholder={inRound ? "A note on this round" : "A message to the agent"}
            value={text}
            disabled={!enabled}
            autoFocus
            onChange={setText}
            onSubmit={onSubmit}
            completion={{ target: { sessionId: session.id }, harness: session.harness, off: !enabled }}
          />
        )}
        {open && codexSkillHint(session.harness, text) && (
          <Text fontSize="xs" color="text.dim">
            {codexSkillHint(session.harness, text)}
          </Text>
        )}
      </ComposerImages>
    </Box>
  );
}
