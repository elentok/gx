import { Box, Text, Textarea } from "@chakra-ui/react";
import { useState } from "react";
import { createPortal } from "react-dom";
import type { SessionInfo, SessionNext, SessionPromptRequest, SessionRoundCard } from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
import { postPrompt, type PostPrompt } from "./prompt";
import { isAnswered, nameOf, roundPrompt, unansweredOf, type Answers } from "./round";
import { promptable } from "./view";

// At these steps Progress links to the implementation epic instead of offering Continue.
const NO_CONTINUE_STEPS = ["branch", "done"];

// Under the chat: the notes box behind a note icon, and Continue after a run-end line. Nothing sends unless the session is Idle.
export function Composer({
  session,
  lastMessage,
  next,
  step,
  nextTitle,
  connected,
  post = postPrompt,
  onStale,
  round = [],
  answers = {},
  onReview,
  fullScreen,
  onFullScreen,
}: {
  session: SessionInfo;
  /** The id of the last chat message the page saw. */
  lastMessage?: string;
  next?: SessionNext;
  /** The Progress step the design is at. */
  step?: string;
  /** Shown under Continue: the map's next ticket title, or the step label. */
  nextTitle?: string;
  connected: boolean;
  post?: PostPrompt;
  /** The conversation moved under the person: refetch the chat, keep the draft. */
  onStale?: () => void;
  /** The open Grilling round: Send answers all of it in one prompt. */
  round?: SessionRoundCard[];
  answers?: Answers;
  /** "Answer them" in the unanswered confirm: open the sheet at that question. */
  onReview?: (index: number) => void;
  /** The round is full screen: the composer renders into this element, its note always open. */
  fullScreen?: HTMLElement | null;
  /** Opens or closes the full-screen round; a sent round closes it. */
  onFullScreen?: (on: boolean) => void;
}) {
  const [open, setOpen] = useState(false);
  const [unanswered, setUnanswered] = useState<SessionRoundCard[]>([]);
  const [text, setText] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const enabled = connected && promptable(session) && !pending;

  const send = async (body: Partial<SessionPromptRequest>, onSent?: () => void) => {
    if (!enabled) return;
    setPending(true);
    setError("");
    try {
      const res = await post({ id: session.id, text: "", last_message: lastMessage, ...body });
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

  const inRound = round.length > 0;
  const sendRound = () => {
    setUnanswered([]);
    return send({ text: roundPrompt(round, answers, text) }, () => {
      setText("");
      onFullScreen?.(false);
    });
  };
  const onSend = () => {
    if (!inRound) return send({ text }, () => setText(""));
    const missing = unansweredOf(round, answers);
    if (missing.length > 0) setUnanswered(missing);
    else sendRound();
  };

  const canContinue = !!next && !NO_CONTINUE_STEPS.includes(step ?? "");
  const sendButton = (
    <Button disabled={!enabled || (!inRound && text.trim() === "")} onClick={onSend} height="auto">
      Send
    </Button>
  );
  const focusButton = inRound && onFullScreen && (
    <Button variant="outline" onClick={() => onFullScreen(true)} height="auto">
      ⤢ Focus
    </Button>
  );
  const notices = (
    <>
      {error && <Text role="alert">{error}</Text>}
      {unanswered.length > 0 && (
        <Box role="alertdialog" aria-label="Unanswered questions" display="flex" flexDirection="column" gap="2">
          <Text>
            {unanswered.length} {unanswered.length === 1 ? "question is" : "questions are"} unanswered (
            {unanswered.map(nameOf).join(", ")}). Send anyway?
          </Text>
          <Box display="flex" gap="2">
            <Button
              variant="outline"
              onClick={() => {
                onReview?.(round.findIndex((c) => c.label === unanswered[0].label));
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

  if (fullScreen) {
    const done = round.filter((c) => isAnswered(answers[c.label])).length;
    return createPortal(
      <Box borderTopWidth="1px" bg="surface" px="4" py="3" display="flex" flexDirection="column" gap="2" role="form" aria-label="Composer">
        {notices}
        <Box display="flex" gap="3" flexDirection={{ base: "column", md: "row" }}>
          <Textarea
            aria-label="Message"
            flex="1"
            rows={2}
            placeholder="Note for the whole round (optional)"
            value={text}
            disabled={!enabled}
            onChange={(e) => setText(e.target.value)}
          />
          <Box display="flex" flexDirection="column" gap="1" justifyContent="flex-end">
            <Text fontSize="xs" color="text.dim" textAlign="center">
              {done} of {round.length} answered
            </Text>
            <Button disabled={!enabled} onClick={onSend} size="md" justifyContent="center" px="6">
              Send
            </Button>
          </Box>
        </Box>
      </Box>,
      fullScreen,
    );
  }

  return (
    <Box position="sticky" bottom="0" bg="surface" borderTopWidth="1px" py="3" display="flex" flexDirection="column" gap="2" role="form" aria-label="Composer">
      {open && (
        // The note is one unit: its box, then Close and Send right under it.
        <Box display="flex" flexDirection="column" gap="2">
          <Textarea aria-label="Message" value={text} disabled={!enabled} onChange={(e) => setText(e.target.value)} autoFocus />
          <Box display="flex" gap="2" justifyContent="flex-end">
            <Button variant="ghost" tone="muted" onClick={() => setOpen(false)}>
              Close
            </Button>
            {focusButton}
            {sendButton}
          </Box>
        </Box>
      )}
      {notices}
      {/* Under the note: its toggle and Continue; a round with no note open sends from here. */}
      <Box display="flex" gap="2" alignItems="stretch">
        {!open && (
          <Button variant="outline" aria-label="Note" onClick={() => setOpen(true)} height="auto">
            ✎ Note
          </Button>
        )}
        {canContinue && (
          <Button disabled={!enabled} onClick={() => send({ continue: true })} flex="1" flexDirection="column" height="auto" py="2">
            <Text as="span">Continue</Text>
            <Text as="span" fontSize="xs" fontWeight="normal">
              {nextTitle}
            </Text>
          </Button>
        )}
        {!open && focusButton}
        {inRound && !open && sendButton}
      </Box>
    </Box>
  );
}
