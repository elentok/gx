import { Box, Text } from "@chakra-ui/react"
import type { DetailRound, TicketDetail } from "../generated/server"
import { Markdown } from "../markdown/Markdown"
import { Card } from "../ui/card/Card"
import { isAnsweredRound, type Sent } from "./answer"
import { AnswerResult } from "./AnswerResult"
import { Collapsible } from "./Collapsible"

const Label = ({ children }: { children: string }) => (
  <Text
    fontFamily="condensed"
    fontSize="xs"
    fontWeight="600"
    textTransform="uppercase"
    color="fg.muted"
  >
    {children}
  </Text>
)

function EarlierRound({ round }: { round: DetailRound }) {
  return (
    <Text>
      #{round.seq} · {round.by} · {round.summary} ({round.outcome})
    </Text>
  )
}

// The reading view, or the compact card once the latest round has an answer.
export function TicketReading({
  detail,
  sent = null,
}: {
  detail: TicketDetail
  sent?: Sent | null
}) {
  const { latest } = detail
  const asker = latest?.by === "gx" ? "GX ASKS" : "THE AGENT ASKS"
  const answered = isAnsweredRound(latest, sent)

  return (
    <Box mt="4" display="flex" flexDirection="column" gap="4">
      {answered && <AnswerResult detail={detail} sent={sent} />}
      {latest && !answered && (
        <>
          <Box>
            <Label>{asker}</Label>
            {latest.text ? (
              <Markdown epic={detail.epic}>{latest.text}</Markdown>
            ) : (
              <Text>{latest.summary}</Text>
            )}
          </Box>
          {latest.handoff && (
            <Collapsible label="Agent's notes">
              <Markdown epic={detail.epic} smallHeadings>
                {latest.handoff}
              </Markdown>
            </Collapsible>
          )}
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
          {latest.prompt && <Text>{latest.prompt}</Text>}
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
  )
}
