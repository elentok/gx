import { Box, chakra } from "@chakra-ui/react"
import { useConnectionContext, useConnectionState } from "../connection"
import { useWide } from "../useWide"
import type { SessionInfo } from "../generated/server"
import { agentDetails } from "./agentDetails"
import { ContextBadge } from "./ContextBadge"

/** The line inside a composer's box: the model on the left, the context badge on the right. */
export function FooterLine({ session }: { session: SessionInfo }) {
  const wide = useWide()
  const ceiling = useConnectionState(useConnectionContext()).snapshot?.context_ceiling
  const { model, effort } = agentDetails(session)
  const text = wide && effort ? `${model} · ${effort}` : model
  // The server sends no context_tokens for a harness other than Claude or Codex.
  const showBadge = session.context_tokens != null && !!ceiling
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
      {showBadge && (
        <ContextBadge
          tokens={session.context_tokens!}
          level={session.context_level!}
          ceiling={ceiling!}
          usage={session.usage}
        />
      )}
    </Box>
  )
}
