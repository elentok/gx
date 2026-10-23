import { Box, Text, Textarea } from "@chakra-ui/react";
import { useState } from "react";
import type { SessionInfo, SessionNext, SessionPromptRequest, SessionRoundCard } from "../generated/server";
import { ReasonStale } from "../ticket/answer";
import { Button } from "../ui/button/Button";
import { postPrompt, type PostPrompt } from "./prompt";
import { roundPrompt, unansweredOf, type Answers } from "./round";

const CONTINUE_PHASES = ["map", "review"];

// Under the chat: the notes box behind a note icon, and Continue after a run-end line. Nothing sends unless the session is Idle.
export function Composer({
  session,
  lastMessage,
  next,
  connected,
  post = postPrompt,
  onStale,
  round = [],
  answers = {},
  onReview,
}: {
  session: SessionInfo;
  /** The id of the last chat message the page saw. */
  lastMessage?: string;
  next?: SessionNext;
  connected: boolean;
  post?: PostPrompt;
  /** The conversation moved under the person: refetch the chat, keep the draft. */
  onStale?: () => void;
  /** The open Grilling round: Send answers all of it in one prompt. */
  round?: SessionRoundCard[];
  answers?: Answers;
  /** "Answer them" in the unanswered confirm: open the sheet at that question. */
  onReview?: (index: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const [unanswered, setUnanswered] = useState<SessionRoundCard[]>([]);
  const [text, setText] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const enabled = connected && session.status.state === "idle" && !pending;

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
    return send({ text: roundPrompt(round, answers, text) }, () => setText(""));
  };
  const onSend = () => {
    if (!inRound) return send({ text }, () => setText(""));
    const missing = unansweredOf(round, answers);
    if (missing.length > 0) setUnanswered(missing);
    else sendRound();
  };

  const phase = next?.phase ?? "";
  return (
    <Box position="sticky" bottom="0" bg="surface" borderTopWidth="1px" py="3" display="flex" flexDirection="column" gap="2" role="form" aria-label="Composer">
      {CONTINUE_PHASES.includes(phase) && (
        <Button disabled={!enabled} onClick={() => send({ continue: true })} flexDirection="column" height="auto" py="2">
          <Text as="span">Continue</Text>
          <Text as="span" fontSize="xs" fontWeight="normal">
            Continue · {phase}
          </Text>
        </Button>
      )}
      {open && (
        <Textarea aria-label="Message" value={text} disabled={!enabled} onChange={(e) => setText(e.target.value)} />
      )}
      {error && <Text role="alert">{error}</Text>}
      {unanswered.length > 0 && (
        <Box role="alertdialog" aria-label="Unanswered questions" display="flex" flexDirection="column" gap="2">
          <Text>
            {unanswered.length} {unanswered.length === 1 ? "question is" : "questions are"} unanswered (
            {unanswered.map((c) => `Q${c.number}`).join(", ")}). Send anyway?
          </Text>
          <Box display="flex" gap="2">
            <Button
              variant="outline"
              onClick={() => {
                onReview?.(round.findIndex((c) => c.number === unanswered[0].number));
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
      <Box display="flex" gap="2">
        <Button variant="outline" aria-label="Note" aria-pressed={open} onClick={() => setOpen((o) => !o)}>
          📝
        </Button>
        {(open || inRound) && (
          <Button disabled={!enabled || (!inRound && text.trim() === "")} onClick={onSend}>
            Send
          </Button>
        )}
      </Box>
    </Box>
  );
}
