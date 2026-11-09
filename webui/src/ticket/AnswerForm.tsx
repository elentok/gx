import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { ActionAnswer, type TicketDetail } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Question } from "../ui/question-sheet/Question";
import { isAnswered, type Answer, type QuestionCard } from "../ui/question-sheet/round";
import { postAnswer, ReasonStale, sentOf, type PostAnswer, type Sent } from "./answer";

// The sticky answer area. The server decides whether it shows (the detail's actions).
export function AnswerForm({
  detail,
  post = postAnswer,
  onSent,
  onStale,
}: {
  detail: TicketDetail;
  post?: PostAnswer;
  onSent?: (sent: Sent) => void;
  /** The question changed under the person: refetch so the current one shows. */
  onStale?: () => void;
}) {
  const { latest } = detail;
  const [answer, setAnswer] = useState<Answer>({});
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [sentSeq, setSentSeq] = useState<number | null>(null);
  const [seenState, setSeenState] = useState(detail.state);

  // A new question clears the pick and the text; the note stays so a stale refusal loses nothing.
  if (seenState !== detail.state) {
    setSeenState(detail.state);
    setAnswer((a) => ({ note: a.note }));
  }

  if (!latest || !detail.actions.includes(ActionAnswer) || sentSeq === latest.seq) return null;

  const isText = latest.kind !== "yes-no" && latest.kind !== "options";
  // A choice question may be answered by a note alone; free text needs its text.
  const ready = isAnswered(answer) || (!isText && !!answer.note?.trim());

  const send = async () => {
    if (!ready || pending) return;
    setPending(true);
    setError("");
    try {
      const body = {
        address: detail.address,
        state: detail.state ?? "",
        ...(answer.huh ? { huh: true } : isText ? { text: answer.text } : answer.choice ? { choice: answer.choice } : {}),
        ...(!isText && answer.note?.trim() ? { note: answer.note } : {}),
      };
      const res = await post(body);
      if ("refused" in res) {
        setError(
          res.reason === "agent-not-ready" ? `${res.message} Approve the agent's prompt first, then send again.` : res.message,
        );
        if (res.reason === ReasonStale) onStale?.();
      } else {
        setSentSeq(latest.seq);
        onSent?.(sentOf(latest.seq, detail.address, res, answer.choice || answer.text));
      }
    } catch {
      setError("Could not send the answer. Try again.");
    } finally {
      setPending(false);
    }
  };

  const card: QuestionCard = {
    label: String(latest.seq),
    title: latest.summary,
    body: latest.text,
    kind: latest.kind ?? "text",
    yes: latest.yes,
    no: latest.no,
    options: latest.options,
    recommended: latest.recommended,
  };

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface.raised"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      aria-label="Answer"
      role="form"
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      borderRadius="12px"
      px="3"
    >
      <Question card={card} answer={answer} onChange={setAnswer} onPicked={() => {}} />
      {error && <Text role="alert">{error}</Text>}
      <Button emphasis="main" alignSelf="flex-end" disabled={!ready || pending} onClick={send}>
        Send answer
      </Button>
    </Box>
  );
}
