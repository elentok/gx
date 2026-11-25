import { Box, Text } from "@chakra-ui/react"
import type { EpicDetail } from "./detail"
import { blockerHint } from "./waiting"

/** Under the header of a Waiting epic: why it does not move, and (later) the blocker chips. */
export function BlockedBy({ detail }: { detail: EpicDetail }) {
  const hint = blockerHint(detail.blockers ?? [])
  if (!hint) return null
  return (
    <Box display="flex" flexDirection="column" mt="4" gap="3">
      <Text
        data-testid="blocked-by-hint"
        fontSize="14px"
        color={hint.stuck ? "tone.paused.text" : "text.dim"}
      >
        {hint.text}
      </Text>
    </Box>
  )
}
