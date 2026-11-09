import { Box, Text } from "@chakra-ui/react";
import { useLayoutEffect, useRef, useState } from "react";
import type { TicketDetail } from "../generated/server";
import { Markdown } from "../markdown/Markdown";
import { Button } from "../ui/button/Button";
import { OutcomeAdopted, OutcomeQueued, type Sent } from "./answer";

const dim = { fontSize: "xs", color: "fg.muted" } as const;

function status(sent: Sent): { head: string; tail: string; tone: "running" | "waiting" | "done" } {
  switch (sent.outcome) {
    case OutcomeQueued:
      return { head: "Queued", tail: sent.position ? `#${sent.position} in queue` : "in queue", tone: "waiting" };
    case OutcomeAdopted:
      return { head: "Sent", tail: "The agent had already finished", tone: "done" };
    default:
      return { head: "Delivered", tail: "Agent is working", tone: "running" };
  }
}

// The answer is the last web message of the exchange (it also covers the fixed "Huh?" message);
// the round's own answer is the fallback for a detail that carries no chat.
function answerText(detail: TicketDetail): string {
  const answer = detail.latest?.answer;
  const fromChat = [...detail.chat].reverse().find((m) => m.from === "web")?.text;
  return fromChat || answer?.choice || answer?.text || "no option picked";
}

// The compact card of an answered round, in place of the reading view. The status line is the send
// outcome this page knows; a reload has none.
export function AnswerResult({ detail, sent }: { detail: TicketDetail; sent: Sent | null }) {
  const latest = detail.latest;
  const [open, setOpen] = useState(false);
  const [cut, setCut] = useState(false);
  const body = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const el = body.current;
    if (el && !open) setCut(el.scrollHeight > el.clientHeight);
  }, [latest?.text, open]);

  if (!latest?.answer) return null;
  const note = latest.answer.note;
  const shown = sent && sent.seq === latest.seq ? status(sent) : null;
  const toggle = cut || !!latest.handoff || open;
  return (
    <Box role="group" aria-label="Answered question" display="flex" flexDirection="column" gap="2">
      <Text fontFamily="condensed" fontSize="xs" fontWeight="600" textTransform="uppercase" color="fg.muted">
        {latest.by === "gx" ? "GX ASKED" : "THE AGENT ASKED"}
      </Text>
      {latest.text && (
        <Box
          ref={body}
          css={open ? undefined : { display: "-webkit-box", WebkitLineClamp: 3, WebkitBoxOrient: "vertical", overflow: "hidden" }}
        >
          <Markdown epic={detail.epic}>{latest.text}</Markdown>
        </Box>
      )}
      {open && latest.handoff && (
        <Markdown epic={detail.epic} smallHeadings>
          {latest.handoff}
        </Markdown>
      )}
      {toggle && (
        <Box>
          <Button emphasis="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? "▾ Show less" : "▸ Show more"}
          </Button>
        </Box>
      )}
      <Text fontWeight="600">{latest.summary}</Text>
      {latest.prompt && latest.prompt !== latest.summary && <Text>{latest.prompt}</Text>}
      <Box display="flex" gap="2" alignItems="baseline" px="2.5" py="2" rounded="md" bg="surface">
        <Text {...dim}>You</Text>
        <Text fontWeight="700">{answerText(detail)}</Text>
      </Box>
      {note && <Text color="fg.muted">{note}</Text>}
      {shown && (
        <Box role="status" aria-label="Answer result" display="flex" alignItems="center" gap="1.5" whiteSpace="nowrap" {...dim}>
          <Box data-testid="status-dot" data-tone={shown.tone} w="2" h="2" rounded="full" bg={`tone.${shown.tone}.fg`} />
          <Text as="span">{shown.head} ·</Text>
          <Text as="span" color={`tone.${shown.tone}.fg`}>
            {shown.tail}
          </Text>
        </Box>
      )}
      {latest.closed_reason && <Text {...dim}>Closed: {latest.closed_reason}</Text>}
    </Box>
  );
}
