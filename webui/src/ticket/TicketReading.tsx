import { Box, Heading, Text } from "@chakra-ui/react";
import { RoundOpen, type DetailRound, type TicketDetail } from "../generated/server";
import { Badge } from "../ui/badge/Badge";
import { Card } from "../ui/card/Card";
import { Collapsible } from "./Collapsible";

const Label = ({ children }: { children: string }) => (
  <Text fontFamily="condensed" fontSize="xs" fontWeight="600" textTransform="uppercase" color="fg.muted">
    {children}
  </Text>
);

function EarlierRound({ round }: { round: DetailRound }) {
  return (
    <Text>
      #{round.seq} · {round.by} · {round.summary} ({round.outcome})
    </Text>
  );
}

// The reading view; the answer area (ticket 49) and permission card (ticket 52) join it later.
export function TicketReading({ detail }: { detail: TicketDetail }) {
  const { latest } = detail;
  const waiting = latest?.outcome === RoundOpen && !detail.answered;
  const asker = latest?.by === "gx" ? "GX ASKS" : "THE AGENT ASKS";

  return (
    <Box mt="4" display="flex" flexDirection="column" gap="4">
      <Box>
        <Badge tone={waiting ? "waiting" : "muted"}>{detail.status}</Badge>
        <Heading as="h1" fontFamily="condensed" size="lg" mt="2">
          {detail.title}
        </Heading>
      </Box>
      {latest && (
        <>
          <Box>
            <Label>{asker}</Label>
            <Text>{latest.text ?? latest.summary}</Text>
          </Box>
          {latest.handoff && <Collapsible label="Agent's notes">{latest.handoff}</Collapsible>}
          {detail.chat.length > 0 && (
            <Box display="flex" flexDirection="column" gap="2">
              {detail.chat.map((m, i) => (
                <Card key={i}>
                  <Label>{m.from}</Label>
                  <Text>{m.text}</Text>
                </Card>
              ))}
            </Box>
          )}
          {latest.closed_reason && <Text>Closed: {latest.closed_reason}</Text>}
        </>
      )}
      {detail.rounds.length > 0 && (
        <Collapsible label={`Earlier rounds (${detail.rounds.length})`}>
          {detail.rounds.map((r) => (
            <EarlierRound key={r.seq} round={r} />
          ))}
        </Collapsible>
      )}
    </Box>
  );
}
