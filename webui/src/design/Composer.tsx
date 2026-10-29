import { Box, Spinner, Text, Textarea } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { useComposerDraft } from "../drafts";
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
import { InterruptPopover } from "./InterruptPopover";
import { NoteBox } from "./NoteBox";
import { postPrompt, type PostPrompt } from "./prompt";
import { roundPrompt, unansweredOf, type Answers } from "./round";
import { SendPopover } from "./SendPopover";
import { useWide } from "../useWide";
import { promptable } from "./view";

// Under the chat, one line: the Auto menu, then Note and the main action. Nothing sends unless the session is Idle.
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
  onRoundSent,
  onReview,
  autoMenu,
  design = true,
  reason,
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
  /** The design-only parts: Auto menu, Continue, Send answers, Interrupt and the Note toggle. Off, the box is always open with Send beside it. */
  design?: boolean;
  /** The line under the box when `design` is off; the caller decides the text. */
  reason?: string;
}) {
  const wide = useWide();
  const [gapsOpen, setGapsOpen] = useState(false);
  const sendRef = useRef<HTMLButtonElement>(null);
  const [interruptOpen, setInterruptOpen] = useState(false);
  const interruptRef = useRef<HTMLButtonElement>(null);
  const [text, setText] = useComposerDraft(session.id);
  // Shared text is never hidden behind a closed Note: it opens on load and on every change (another tab).
  const [open, setOpen] = useState(text !== "");
  useEffect(() => {
    if (text !== "") setOpen(true);
  }, [text]);
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
  const onInterrupt = () => {
    setInterruptOpen(false);
    return run(() => interrupt({ id: session.id, state: stateToken(session) }));
  };

  const inRound = round.length > 0;
  const sendRound = () => {
    setGapsOpen(false);
    return send({ text: roundPrompt(round, answers, text) }, () => {
      setText("");
      onRoundSent?.();
    });
  };
  const unanswered = unansweredOf(round, answers);
  // With gaps, Send's click opens the popover instead of sending; a click while it is open closes it.
  const onSendRound = () => {
    if (gapsOpen) setGapsOpen(false);
    else if (unanswered.length > 0) setGapsOpen(true);
    else sendRound();
  };

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
  ) : canContinue && !inRound && text.trim() === "" ? (
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
      <Button ref={sendRef} disabled={!enabled} onClick={onSendRound}>
        {pending && <Spinner size="sm" />}
        Send answers
      </Button>
      <SendPopover
        open={gapsOpen && unanswered.length > 0}
        unanswered={unanswered}
        total={round.length}
        sendDisabled={!enabled}
        anchorRef={sendRef}
        onSendAnyway={sendRound}
        onAnswer={(c) =>
          onReview?.(round.findIndex((r) => r.label === c.label))
        }
        onClose={() => setGapsOpen(false)}
      />
    </>
  ) : (
    <Button
      disabled={!enabled || text.trim() === ""}
      onClick={() => send({ text }, () => setText(""))}
    >
      {pending && <Spinner size="sm" />}
      Send
    </Button>
  );

  if (!design) {
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
        <Box display="flex" gap="2" alignItems="flex-end">
          <Textarea
            flex="1"
            fontSize="16px"
            color="text"
            bg="surface"
            borderColor="border"
            borderRadius="12px"
            rows={3}
            resize="none"
            aria-label="Message"
            placeholder="A message to the agent"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <Button
            disabled={!enabled || text.trim() === ""}
            onClick={() => send({ text }, () => setText(""))}
          >
            {pending && <Spinner size="sm" />}
            Send
          </Button>
        </Box>
        {reason && (
          <Text fontSize="xs" color="text.dim">
            {reason}
          </Text>
        )}
      </Box>
    );
  }

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
        />
      )}
      {/* One line: the Auto menu on the left, the main action on the right. */}
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
