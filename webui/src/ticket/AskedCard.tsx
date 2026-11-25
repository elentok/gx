import { Box, Text } from "@chakra-ui/react"
import { useNavigate } from "react-router"
import type { TicketDetail } from "../generated/server"
import { answerPath } from "../hrefs"
import { Button } from "../ui/button/Button"
import { RecoveryActions } from "./RecoveryActions"
import { askerLabel, Label } from "./TicketReading"

// The compact card of an answerable ticket: who asks, the title, and one row of buttons. The body
// is on the answer page.
export function AskedCard({
  detail,
  onDone,
  onStale,
}: {
  detail: TicketDetail
  onDone: () => void
  onStale: () => void
}) {
  const navigate = useNavigate()
  const { latest } = detail
  return (
    <Box
      display="flex"
      flexDirection="column"
      gap="3"
      p="3"
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      rounded="xl"
      bg="surface.raised"
    >
      <Box>
        <Label>{askerLabel(latest)}</Label>
        <Text fontSize="lg" fontWeight="600" mt="1">
          {latest?.summary}
        </Text>
        {latest?.prompt && <Text>{latest.prompt}</Text>}
      </Box>
      <RecoveryActions
        detail={detail}
        onDone={onDone}
        onStale={onStale}
        leading={
          <Button emphasis="main" onClick={() => navigate(answerPath(detail.address))}>
            Answer question
          </Button>
        }
      />
    </Box>
  )
}
