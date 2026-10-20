import { Box, Text } from "@chakra-ui/react";
import type { TicketDetail } from "../generated/server";
import { Badge } from "../ui/badge/Badge";
import { OutcomeAdopted, OutcomeDelivered, OutcomeQueued, type Sent } from "./answer";

function sentWords(sent: Sent): string {
  switch (sent.outcome) {
    case OutcomeQueued:
      return sent.position ? `Answer sent · queued, #${sent.position} in queue` : "Answer sent · queued";
    case OutcomeAdopted:
      return "Answer sent · the agent had already finished";
    default:
      return "Answer sent · delivered";
  }
}

// The result of a send, read back from the refetched detail. It carries no controls: an answer
// cannot be withdrawn from the page.
export function AnswerResult({ detail, sent }: { detail: TicketDetail; sent: Sent | null }) {
  if (!sent || detail.latest?.seq !== sent.seq) return null;
  const answer = detail.latest.answer;
  return (
    <Box role="status" aria-label="Answer result" display="flex" flexDirection="column" gap="1" mt="4">
      <Box display="flex" gap="2" alignItems="center">
        <Badge tone="waiting">{sent.outcome === OutcomeQueued ? "Answered" : "Sent"}</Badge>
        <Text>{sentWords(sent)}</Text>
        {sent.outcome === OutcomeDelivered && <Badge tone="running">Agent is working</Badge>}
      </Box>
      {answer && (answer.choice || answer.text) && <Text>{answer.choice ?? answer.text}</Text>}
      {answer?.note && <Text color="fg.muted">{answer.note}</Text>}
    </Box>
  );
}
