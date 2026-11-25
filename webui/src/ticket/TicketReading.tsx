import { Box, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import type { DetailRound, TicketDetail } from "../generated/server"
import { Markdown } from "../markdown/Markdown"
import { Card } from "../ui/card/Card"
import { withKeys } from "../uniqueKeys"
import { isAnsweredRound, showsSentCard, type Sent } from "./answer"
import { AnswerResult } from "./AnswerResult"
import { Collapsible } from "./Collapsible"

export const askerLabel = (latest: { by?: string } | null | undefined) =>
  latest?.by === "gx" ? "GX ASKS" : "THE AGENT ASKS"

export const Label = ({ children }: { children: string }) => (
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
  card,
}: {
  detail: TicketDetail
  sent?: Sent | null
  /** The compact question card of an answerable ticket: it replaces the question text and the chat. */
  card?: ReactNode
}) {
  const { latest } = detail
  const asker = askerLabel(latest)
  const answered = isAnsweredRound(detail, sent)

  return (
    <Box mt="4" display="flex" flexDirection="column" gap="4">
      {answered && latest?.handoff && showsSentCard(latest, sent) && (
        <Collapsible label="Agent's notes">
          <Markdown epic={detail.epic} smallHeadings>
            {latest.handoff}
          </Markdown>
        </Collapsible>
      )}
      {answered && <AnswerResult detail={detail} sent={sent} />}
      {latest && !answered && (
        <>
          {!card && (
            <Box>
              <Label>{asker}</Label>
              {latest.text ? (
                <Markdown epic={detail.epic}>{latest.text}</Markdown>
              ) : (
                <Text>{latest.summary}</Text>
              )}
            </Box>
          )}
          {latest.handoff && (
            <Collapsible label="Agent's notes">
              <Markdown epic={detail.epic} smallHeadings>
                {latest.handoff}
              </Markdown>
            </Collapsible>
          )}
          {card}
          {!card && detail.chat.length > 0 && (
            <Box display="flex" flexDirection="column" gap="2">
              {withKeys(detail.chat, (m) => `${m.from}|${m.text}`).map(({ item: m, key }) => (
                <Card key={key}>
                  <Label>{m.from}</Label>
                  <Text>{m.text}</Text>
                </Card>
              ))}
            </Box>
          )}
          {!card && latest.prompt && <Text>{latest.prompt}</Text>}
          {latest.closed_reason && <Text>Closed: {latest.closed_reason}</Text>}
        </>
      )}
      <EarlierRounds detail={detail} muted={answered && !!latest && showsSentCard(latest, sent)} />
    </Box>
  )
}

export function EarlierRounds({ detail, muted }: { detail: TicketDetail; muted?: boolean }) {
  if (detail.rounds.length === 0) return null
  return (
    <Collapsible muted={muted} label={`Earlier rounds (${detail.rounds.length})`}>
      {detail.rounds.map((r) => (
        <EarlierRound key={r.seq} round={r} />
      ))}
    </Collapsible>
  )
}
