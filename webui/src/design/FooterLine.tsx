import { Box, chakra } from "@chakra-ui/react"
import { useWide } from "../useWide"
import type { SessionInfo } from "../generated/server"
import { agentDetails } from "./agentDetails"
import { ContextBadge } from "./ContextBadge"

/** The line inside a composer's box: the model on the left, the context badge on the right. */
export function FooterLine({ session }: { session: SessionInfo }) {
  const wide = useWide()
  const { model, effort } = agentDetails(session)
  const text = wide && effort ? `${model} · ${effort}` : model
  // The server sends no context reading for a harness other than Claude or Codex.
  const { context_tokens: tokens, context_level: level, context_ceiling: ceiling } = session
  return (
    <Box
      data-testid="footer-line"
      display="flex"
      alignItems="center"
      justifyContent="space-between"
      gap="2"
      fontSize="xs"
      color="text.dim"
    >
      <chakra.span truncate>{text}</chakra.span>
      {tokens != null && level && ceiling && (
        <ContextBadge tokens={tokens} level={level} ceiling={ceiling} usage={session.usage} />
      )}
    </Box>
  )
}
