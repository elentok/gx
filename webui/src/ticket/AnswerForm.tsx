import { Box, Text, Textarea } from "@chakra-ui/react";
import { useState } from "react";
import { ActionAnswer, type DetailOption, type TicketDetail } from "../generated/server";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { postAnswer, ReasonStale, sentOf, type PostAnswer, type Sent } from "./answer";

interface Choice {
  id: string;
  label: string;
  consequence?: string;
}

function choicesOf(latest: NonNullable<TicketDetail["latest"]>): Choice[] {
  if (latest.kind === "yes-no") {
    return [
      { id: "yes", label: "Yes", consequence: latest.yes },
      { id: "no", label: "No", consequence: latest.no },
    ];
  }
  return (latest.options ?? []).map((o: DetailOption) => o);
}

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
  const [choice, setChoice] = useState("");
  const [text, setText] = useState("");
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [sentSeq, setSentSeq] = useState<number | null>(null);
  const [seenState, setSeenState] = useState(detail.state);

  // A new question clears the pick and the text; the note stays so a stale refusal loses nothing.
  if (seenState !== detail.state) {
    setSeenState(detail.state);
    setChoice("");
    setText("");
  }

  if (!latest || !detail.actions.includes(ActionAnswer) || sentSeq === latest.seq) return null;

  const isText = latest.kind !== "yes-no" && latest.kind !== "options";
  const ready = isText ? text.trim() !== "" : choice !== "";

  const send = async () => {
    if (!ready || pending) return;
    setPending(true);
    setError("");
    try {
      const body = {
        address: detail.address,
        state: detail.state ?? "",
        ...(isText ? { text } : { choice }),
        ...(!isText && note.trim() ? { note } : {}),
      };
      const res = await post(body);
      if ("refused" in res) {
        setError(
          res.reason === "agent-not-ready" ? `${res.message} Approve the agent's prompt first, then send again.` : res.message,
        );
        if (res.reason === ReasonStale) onStale?.();
      } else {
        setSentSeq(latest.seq);
        onSent?.(sentOf(latest.seq, detail.address, res));
      }
    } catch {
      setError("Could not send the answer. Try again.");
    } finally {
      setPending(false);
    }
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
    >
      {isText ? (
        <Textarea
          aria-label="Your answer"
          placeholder="Your answer"
          value={text}
          disabled={pending}
          onChange={(e) => setText(e.target.value)}
        />
      ) : (
        choicesOf(latest).map((c) => (
          <Button
            key={c.id}
            emphasis={choice === c.id ? "main" : "secondary"}
            aria-pressed={choice === c.id}
            disabled={pending}
            onClick={() => setChoice(c.id)}
            width="full"
            height="auto"
            py="3"
            justifyContent="flex-start"
            flexDirection="column"
            alignItems="flex-start"
          >
            <Text as="span">
              {c.label}
              {latest.recommended === c.id && (
                <>
                  {" "}
                  <Badge tone="waiting">Recommended</Badge>
                </>
              )}
            </Text>
            {c.consequence && (
              <Text as="span" fontSize="sm" fontWeight="normal" whiteSpace="normal" textAlign="start">
                {c.consequence}
              </Text>
            )}
          </Button>
        ))
      )}
      {/* A free-text answer is the note already. */}
      {!isText && (
        <Textarea
          aria-label="Note (optional)"
          placeholder="Note (optional)"
          value={note}
          disabled={pending}
          onChange={(e) => setNote(e.target.value)}
        />
      )}
      {error && <Text role="alert">{error}</Text>}
      <Button emphasis="main" alignSelf="flex-end" disabled={!ready || pending} onClick={send}>
        Send answer
      </Button>
    </Box>
  );
}
