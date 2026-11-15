import { Box, chakra } from "@chakra-ui/react"
import { useWide } from "../useWide"
import type { SessionInfo } from "../generated/server"
import { modelName } from "../modelName"
import { agentDetails } from "./agentDetails"
import { ContextBadge } from "./ContextBadge"
import { openModelPicker } from "../shell/modelPickerOpen"
import { switchRunning } from "./modelSwitch"
import { OutdatedModel } from "./OutdatedModel"

/** The line inside a composer's box: the model on the left, the context badge on the right. */
export function FooterLine({ session }: { session: SessionInfo }) {
  const wide = useWide()
  const switching = switchRunning(session)
  // While a switch runs, the target stands in for the old model and its effort.
  const { model, effort } = switching
    ? {
        model: modelName(session.switching_model ?? "", session.harness),
        effort: session.switching_effort ?? "",
      }
    : agentDetails(session)
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
      <chakra.span truncate>
        {session.latest_model && !switching ? (
          <OutdatedModel session={session} text={text} />
        ) : !session.design && !switching ? (
          <chakra.button
            type="button"
            data-testid="model-name-button"
            borderWidth="0"
            bg="transparent"
            p="0"
            font="inherit"
            color="inherit"
            cursor="pointer"
            _hover={{ textDecoration: "underline" }}
            onClick={() => openModelPicker({ session })}
          >
            {text}
          </chakra.button>
        ) : (
          text
        )}
        {switching && <chakra.span data-testid="switching-hint"> · switching</chakra.span>}
      </chakra.span>
      {tokens != null && level && ceiling && (
        <ContextBadge tokens={tokens} level={level} ceiling={ceiling} usage={session.usage} />
      )}
    </Box>
  )
}
